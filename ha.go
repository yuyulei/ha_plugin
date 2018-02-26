package ha

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"fmt"

	"strconv"
	"strings"

	log "github.com/Sirupsen/logrus"
	consulcli "qiniu.com/dora-cloud/utils/consulclient"
)

type HA interface {
	Register(*Service)
	Service() *Service
	Leader() *Service
	UpdateLeader(*Service)
	IsLeader() bool

	Handler()

	// Setup including:
	// 1. register health check
	// 1.5 wait check passing
	// 2. create session
	Setup() error

	// Start including:
	// 1. hearbeat
	// 	(1) acquire kv with session
	// 	(2) get/update leader info
	Start()

	// Stop including:
	// 1. release session
	// 2. deregister health check
	Stop()
}

func NewHAManager(consulCfg *ConsulConfig, hamanagerCfg *HAManagerConfig) *HAManager {
	config := consulcli.ConsulClientConfig{
		Region: consulCfg.Region,
		Addrs:  []string{consulCfg.Addr},
	}
	cli := consulcli.NewConsulClient(config)

	return &HAManager{
		cli:        cli,
		defaultCli: http.DefaultClient,

		health:      hamanagerCfg.HealthCheck,
		httpHeaders: hamanagerCfg.HttpHeaders,
	}
}

// 一些与 consul 紧密相关的功能函数
func (m *HAManager) CreateSession() error {
	var (
		serviceName = m.Service().Name
		checkId     = m.GetCheckId()
	)

	sid, err := m.cli.CreateSession(serviceName, checkId)
	if err != nil {
		log.Errorf("failed to create session, err: %s\n", err)
		return err
	}
	m.SetSessionId(sid)
	log.Infof("succeed to update session id[%s]", sid)
	return nil
}

func (m *HAManager) ReleaseSession() error {
	var (
		serviceName = m.Service().Name
		sid         = m.GetSessionId()
		svc         = &Service{}
	)
	svc = m.Service()
	data, err := json.Marshal(svc)
	if err != nil {
		log.Errorf("fail to marshal leader info, err: %s\n", err)
		return err
	}

	flag, err := m.cli.ReleaseKV(serviceName, data, sid)
	if err != nil {
		log.Errorf("fail to release kv[%s] from session[%s]", serviceName, sid)
		return err
	}
	log.Debugf("succeed to call release kv, flag[%t]", flag)

	if flag {
		return nil
	} else {
		log.Errorf("fail to release kv[%s] from session[%s], maybe you not acquire it", serviceName, sid)
		// TODO: 指定一个 error
		return errors.New("you not acquire it")
	}
}

func (m *HAManager) RegisterHealthCheck() (err error) {
	var (
		serviceName = m.Service().Name
		url         = m.GetHealthHost() + m.GetHealthPath()
		timeout     = m.GetHealthTimeout()
		interval    = m.GetHealthInterval()
	)
	checkId, err := m.cli.RegisterCheck(serviceName, url, timeout, interval)
	if err != nil {
		log.Errorf("fait to register check, err: %s", err)
	}
	m.UpdateCheckId(checkId)
	return nil
}

func (m *HAManager) DeregisterHealthCheck() (err error) {
	var (
		checkId = m.GetCheckId()
	)
	err = m.cli.DeregisterCheck(checkId)
	if err != nil {
		log.Errorf("fait to deregister check[%s], err: %s", checkId, err)
	}
	m.UpdateCheckId("")
	return nil
}

// 一些接口函数
func (m *HAManager) Register(s *Service) {
	m.SetService(s)
}

func (m *HAManager) Service() (s *Service) {
	return m.GetService()
}

func (m *HAManager) Leader() *Service {
	return m.GetLeader()
}

func (m *HAManager) UpdateLeader(s *Service) {
	m.SetLeader(s)
}

func (m *HAManager) IsLeader() bool {
	svc := m.GetService()
	leader := m.GetLeader()
	if svc == nil || leader == nil {
		return false
	}

	if svc.Ip == leader.Ip && svc.Port == leader.Port {
		return true
	} else {
		return false
	}
}

func (m *HAManager) Heartbeat() {
	defer m.WaitGroup.Done()

	var (
		svcLocal  *Service = m.GetService()
		svcLeader *Service = &Service{}
	)

	data, err := json.Marshal(svcLocal)
	if err != nil {
		log.Errorf("fail to marshal leader info, err: %s\n", err)
		return
	}

	for {
		time.Sleep(3 * time.Second)

		if m.shouldStop == 1 {
			break
		}

		// 创建 kv（value: data）并锁住
		suc, err := m.cli.AcquireKV(m.Service().Name, data, m.GetSessionId())
		if err != nil {
			log.Errorf("fail to acquire kv, err: %s\n", err)
			// 关于这个 session 失效的情形:
			// 1. 主动型: 比如说 人为 release session
			// 2. 被动型: 比如说 该 session 对应的健康检查失败, 那么因为 lock-delay 的存在（默认15秒内）, 这个
			// session 不会被强占, 所以你 acquire 的话都会失败, 返回 suc 为false. 如果健康检查能恢复, 则没有异常.
			// 如果不能恢复, 那么允许被其他 session 强占, 但是那个失效的 session 就不能再用了. consul 已经认为它是
			// 一个废弃的 session 了, 所以我们要重新创建 session .
			if err == consulcli.ErrNotFoundSession {
				log.Infof("the old session[%s] is invalid, we will create new one replace it", m.GetSessionId())
				cErr := m.CreateSession()
				if cErr != nil {
					log.Errorf("fail to create a new session, err[%s]", cErr)
				}
			}

			continue
		} else {
			if suc {
				log.Infof("succeed to become leader, update leader service, svc[%s]", svcLocal)
				m.UpdateLeader(svcLocal)
			} else {
				// 什么时候 suc == false?
				// 1. 该 kv 被其他 session 占有
				// 2. 该 session 健康检查失败或其他原因失效（可能会直接destroy）, 但仍处于在 lock-delay 阶段
				log.Debugf("fail to become leader, suc[%t]", suc)

				pair, err := m.cli.GetKV(m.Service().Name)
				if err != nil {
					log.Errorf("fail to get kv, err: %s\n", err)
					continue
				}

				if pair == nil {
					log.Errorf("kv not found")
					continue
				}

				err = json.Unmarshal(pair.Value, svcLeader)
				if err != nil {
					log.Errorf("fail to unmarshal svc, value[%s] err: %s\n", pair.Value, err)
					continue
				}
				log.Infof("leader info [%#v]", svcLeader)
				m.UpdateLeader(svcLeader)
			}
		}
	}
}

func (m *HAManager) Handler(w http.ResponseWriter, r *http.Request, f func(http.ResponseWriter, *http.Request)) {

	switch {
	case r.URL.Path == m.GetHealthPath():
		f(w, r)
	case m.IsLeader():
		f(w, r)
	default:
		w.Header().Add("Content-Type", "application/json")
		// 这里返回的 error 沿用 sdk 使用的 HttpError

		leader := m.Leader()
		if leader == nil {
			log.Error("leader info is nil")
			w.WriteHeader(http.StatusInternalServerError)
			b, err := json.Marshal(ErrFailToGetValidLeaderInfo)
			if err != nil {
				log.Errorf("fail to marshal ha error, err: %s", err)
			}
			w.Write(b)
			return
		}

		host := fmt.Sprintf("%s:%d", leader.Ip, leader.Port)
		newR := rebuildRequest(host, r, m.httpHeaders.Request)
		resp, err := m.defaultCli.Do(newR)
		if err != nil {
			log.Errorf("fail to  get response from leader, err: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			b, err := json.Marshal(ErrFailToConnectToLeader)
			if err != nil {
				log.Errorf("fail to marshal ha error, err: %s", err)
			}
			w.Write(b)
			return
		}

		defer resp.Body.Close()
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Errorf("fail to read body, err: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			b, err := json.Marshal(ErrFailToReadRespBodyFromLeader)
			if err != nil {
				log.Errorf("fail to marshal ha error, err: %s", err)
			}
			w.Write(b)
			return
		}

		//TODO: 添加一些必要的 header, 到 *.conf
		for _, key := range m.httpHeaders.Response {
			if ct := resp.Header.Get(key); ct != "" {
				w.Header().Add(key, ct)
			}
		}

		w.WriteHeader(resp.StatusCode)
		w.Write(data)
		return
	}
}

func (m *HAManager) Setup() error {
	err := m.RegisterHealthCheck()
	if err != nil {
		return nil
	}
	// 只有当 session 中的健康检查成功的时候 才能成功创建 session
	// 而刚刚注册的 check 需要一定的时间才能更新状态, 刚上来是 critical
	wait, err := strconv.Atoi(strings.Split(m.GetHealthInterval(), "s")[0])
	if err != nil {
		return err
	}
	// 所以多给 1 秒, 确保至少有一次健康检查
	wait += 1
	time.Sleep(time.Duration(wait) * time.Second)
	err = m.CreateSession()
	if err != nil {
		return err
	}

	return nil
}

func (m *HAManager) Start() {
	m.WaitGroup.Add(1)
	go m.Heartbeat()
}

func (m *HAManager) Stop() {
	atomic.StoreInt32(&m.shouldStop, 1)
	m.WaitGroup.Wait()

	// 计算 release session 仍在 consul 那边会有一个 lock-delay 的间隔
	// 所以在 Stop() 最好不要立刻结束进程(比如说那些 api 接口), 在 lock-delay 时间里仍应该支持访问
	err := m.ReleaseSession()
	if err != nil {
		log.Errorf("fail to release session, err: %s\n", err)
	} else {
		log.Info("release session done")
	}

	err = m.DeregisterHealthCheck()
	if err != nil {
		log.Errorf("fail to deregister health check, err: %s", err)
	} else {
		log.Info("deregister health check done")
	}

	log.Info("ha: gracefully shut down")
}

func rebuildRequest(host string, r *http.Request, headers []string) *http.Request {
	// 构建新的 url
	newUrl := &url.URL{
		Scheme:   "http",
		Host:     host,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
	}

	// copy 原来的 body pramas
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Errorf("fail to read request body, err: %s", err)
	}
	defer r.Body.Close()
	body := bytes.NewBuffer(data)

	// 构建新的 request
	newRequest, err := http.NewRequest(r.Method, newUrl.String(), body)
	if err != nil {
		log.Errorf("fail to build new request, err: %s", err)
	}

	// 填充必要的 header
	for _, key := range headers {
		if ct := r.Header.Get(key); ct != "" {
			newRequest.Header.Add(key, ct)
		}
	}

	// TODO: 可能需要记录一些七牛特有的 header, 补充到 *.conf
	return newRequest
}

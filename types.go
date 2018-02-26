package ha

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	consulcli "qiniu.com/dora-cloud/utils/consulclient"
)

type Service struct {
	Name string `json:"name"`
	Ip   string `json:"ip"`
	Port int    `json:"port"`
}

func (s *Service) String() string {
	return `{Name: ` + s.Name + `, Ip: ` + s.Ip + `, Port: ` + fmt.Sprintf("%d", s.Port) + `}`
}

type HAManagerConfig struct {
	*HealthCheck `json:"health_check"`
	*HttpHeaders `json:"http_headers"`
}

func (h *HAManagerConfig) String() string {
	return `{HealthCheck: ` + h.HealthCheck.String() + `, HttpHeaders: ` + h.HttpHeaders.String() + `}`
}

type HealthCheck struct {
	Id       string
	Host     string `json:"host"`
	Path     string `json:"path"`
	Timeout  string `json:"timeout"`
	Interval string `json:"interval"`
}

func (h *HealthCheck) String() string {
	return `{Host: ` + h.Host + `, Path: ` + h.Path + `, Timeout: ` + h.Timeout + `, Interval: ` + h.Interval + `}`
}

type HttpHeaders struct {
	Request  []string `json:"request"`
	Response []string `json:"response`
}

func (h *HttpHeaders) String() string {
	return `{Request: [` + strings.Join(h.Request, ", ") + `], Response: [` + strings.Join(h.Response, ", ") + `]}`
}

type ConsulConfig struct {
	Region string `json:"region"`
	Addr   string `json:"addr"`
}

func (c *ConsulConfig) String() string {
	return `{Region: ` + c.Region + `, Addr: ` + c.Addr + `}`
}

type HAManager struct {
	local     *Service
	leader    *Service
	sessionId string

	health      *HealthCheck
	httpHeaders *HttpHeaders

	cli        *consulcli.ConsulClient
	defaultCli *http.Client

	sync.Mutex
	sync.WaitGroup
	shouldStop int32
}

func (m *HAManager) String() string {
	var str string
	if m.local != nil {
		str += `"Local": { ` + m.local.String() + "}"
	} else {
		str += `"Local": {}`
	}
	if m.leader != nil {
		str += `, "Leader": { ` + m.leader.String() + "}"
	} else {
		str += `, "Leader": {}`
	}

	str += `, "SessionId": ` + m.sessionId
	str += `, "Request Header List": {` + strings.Join(m.httpHeaders.Request, ", ") + `}`
	str += `, "Response Header List": {` + strings.Join(m.httpHeaders.Response, ", ") + `}`

	return str
}

func (m *HAManager) SetService(s *Service) {
	m.Lock()
	defer m.Unlock()
	m.local = s
}

func (m *HAManager) GetService() (s *Service) {
	m.Lock()
	defer m.Unlock()
	s = m.local
	return
}

func (m *HAManager) SetLeader(s *Service) {
	m.Lock()
	defer m.Unlock()
	m.leader = s
}

func (m *HAManager) GetLeader() (s *Service) {
	m.Lock()
	defer m.Unlock()
	s = m.leader
	return
}

func (m *HAManager) SetSessionId(sid string) {
	m.Lock()
	defer m.Unlock()
	m.sessionId = sid
}

func (m *HAManager) GetSessionId() (sid string) {
	m.Lock()
	defer m.Unlock()
	sid = m.sessionId
	return
}

func (m *HAManager) GetHealthHost() (host string) {
	m.Lock()
	defer m.Unlock()
	host = m.health.Host
	return
}

func (m *HAManager) GetHealthPath() (path string) {
	m.Lock()
	defer m.Unlock()
	path = m.health.Path
	return
}

func (m *HAManager) GetHealthTimeout() (timeout string) {
	m.Lock()
	defer m.Unlock()
	timeout = m.health.Timeout
	return
}

func (m *HAManager) GetHealthInterval() (interval string) {
	m.Lock()
	defer m.Unlock()
	interval = m.health.Interval
	return
}

func (m *HAManager) UpdateCheckId(checkId string) {
	m.Lock()
	defer m.Unlock()
	m.health.Id = checkId
}

func (m *HAManager) GetCheckId() (checkId string) {
	m.Lock()
	defer m.Unlock()
	checkId = m.health.Id
	return
}

func (m *HAManager) GetRequestHeaders() (list []string) {
	m.Lock()
	defer m.Unlock()
	list = m.httpHeaders.Request
	return
}

func (m *HAManager) GetResponseHeaders() (list []string) {
	m.Lock()
	defer m.Unlock()
	list = m.httpHeaders.Response
	return
}

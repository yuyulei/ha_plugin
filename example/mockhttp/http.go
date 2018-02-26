package mockhttp

import (
	"errors"
	"fmt"

	log "github.com/Sirupsen/logrus"
)

type MockCrtl struct {
	health bool
}

func NewMockCrtl() *MockCrtl {
	return &MockCrtl{
		health: true,
	}
}

type requestArgs struct {
	Xx     string `json:"xx"`
	Yy     int    `json:"yy"`
	Health bool   `json:"health"`
}

type HelloMsg struct {
	Msg string `json:"msg"`
	Ok  bool   `json:"ok"`
}

func (m *MockCrtl) GetHello(args *requestArgs) (HelloMsg, error) {
	log.Infof("receive GetHello, args[%#v]", args)
	msg := HelloMsg{
		Msg: "aaaaaaa",
		Ok:  true,
	}
	return msg, nil
}

func (m *MockCrtl) PostHi(args *requestArgs) (HelloMsg, error) {
	log.Infof("receive PostHi, args[%#v]", args)
	msg := HelloMsg{
		Msg: fmt.Sprintf("%s:%d", args.Xx, args.Yy),
		Ok:  false,
	}
	return msg, nil
}

func (m *MockCrtl) GetError(args *requestArgs) error {
	log.Infof("receive GetError, args[%#v]", args)
	err := errors.New("i am an error!")
	return err
}

// Mock Health Check
func (m *MockCrtl) GetHealth(args *requestArgs) error {
	if m.health {
		return nil
	}
	return errors.New("I'm not healthy")
}

func (m *MockCrtl) PostHealth(args *requestArgs) error {
	m.health = args.Health
	return nil
}

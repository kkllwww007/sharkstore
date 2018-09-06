package alarm2

import (
	"fmt"
	"errors"
	"net"
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"github.com/gomodule/redigo/redis"
	"database/sql"

	"model/pkg/alarmpb2"
	"sync"
	"net/http"
)

type appMap				map[string]TableApp
type appClusterMap 		map[int64]appMap

type globalRuleMap 		map[string]TableGlobalRule

type ruleClusterNameMap	map[string]TableClusterRule
type ruleClusterMap		map[int64]ruleClusterNameMap

type receiverMap 		map[string]TableReceiver
type receiverClusterMap map[int64]receiverMap

type Server struct {
	conf *Alarm2ServerConfig

	jimClient 	*redis.Pool
	mysqlClient *sql.DB
	reportClient *http.Client

	context context.Context

	clusterApp 		appClusterMap
	globalRule 		globalRuleMap
	clusterRule 	ruleClusterMap
	clusterReceiver receiverClusterMap
	appLock			sync.RWMutex
	globalRuleLock 	sync.RWMutex
	clusterRuleLock sync.RWMutex
	receiverLock 	sync.RWMutex
}

func newServer(conf *Alarm2ServerConfig) (*Server, error) {
	s := new(Server)

	s.conf = conf

	var err error
	s.jimClient = s.newJimClient()
	s.mysqlClient, err = s.newMysqlClient()
	if err != nil {
		return nil, err
	}
	s.reportClient = &http.Client{}
	s.context = context.Background()

	return s, nil
}

func NewAlarmServer2(conf *Alarm2ServerConfig) (*Server, error) {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%v", conf.ServerPort))
	if err != nil {
		return nil, err
	}
	s := grpc.NewServer()
	server, err := newServer(conf)
	if err != nil {
		return nil, err
	}

	// register alarm server
	alarmpb2.RegisterAlarmServer(s, server)
	reflection.Register(s)
	go s.Serve(lis) // rpc server

	go server.timingDbPulling()
	go server.aliveChecking()
	return server, nil
}

func (s *Server) Alarm(ctx context.Context, req *alarmpb2.AlarmRequest) (*alarmpb2.AlarmResponse, error) {
	header := req.GetHeader()
	switch req.GetHeader().GetType() {
	case alarmpb2.AlarmType_APP_HEARTBEAT:
		r := req.GetAppHeartbeat()
		if r == nil {
			return nil, errors.New("request AppHeartbeat is nil")
		}
		return s.handleAppHeartbeat(header, r)
	case alarmpb2.AlarmType_RULE_ALARM:
		return s.handleRuleAlarm(header, req.GetRuleAlarm())
	//case alarmpb2.AlarmType_APP_NOT_ALIVE:
	//	r := req.GetAppNotAlive()
	//	if r == nil {
	//		return nil, errors.New("request AppNotAliveAlarm is nil")
	//	}
	//	return s.handleAppNotAlive(header, r)
	//case alarmpb2.AlarmType_GATEWAY_SLOWLOG:
	//	r := req.GetGwSlowLog()
	//	if r == nil {
	//		return nil, errors.New("request GwSlowLog is nil")
	//	}
	//	return s.handleGatewaySlowLog(header, r)
	//case alarmpb2.AlarmType_GATEWAY_ERRORLOG:
	//	r := req.GetGwErrorLog()
	//	if r == nil {
	//		return nil, errors.New("request GwErrorLog is nil")
	//	}
	//	return s.handleGatewayErrorLog(header, r)
	default:
		return nil, errors.New("unknown alarm type")
	}
}


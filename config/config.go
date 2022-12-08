package config

import (
	"errors"
	"github.com/filecoin-project/go-address"
	logging "github.com/ipfs/go-log/v2"
	"net"
	"os"
	"strings"
	"time"
)

var log = logging.Logger("config")

type LotusOpt struct {
	LotusToken      string
	LotusAddress    string
	MinerToken      string
	MinerAddress    string
	Miners          map[address.Address]string
	Wallets         map[address.Address]string
	EnableDeadlines bool
}

func DefaultOpt() *LotusOpt {

	lotusAddr, lotusToken, err := GetApiInfoFromEnv("FULLNODE_API_INFO")
	if err != nil {
		log.Warnf("full node api info get err:%v", err)
		//lotusToken = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJBbGxvdyI6WyJyZWFkIiwid3JpdGUiLCJzaWduIiwiYWRtaW4iXX0.73tEqgoUYz-GH-Z46yZQqv4WTg8CkkWmfmQHAICYo3s"
		//lotusAddr = "121.204.248.35:51234"
	}

	minerAddr, minerToken, err := GetApiInfoFromEnv("MINER_API_INFO")
	if err != nil {
		log.Warnf("miner api info get err:%v", err)
		//minerToken = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJBbGxvdyI6WyJyZWFkIiwid3JpdGUiLCJzaWduIiwiYWRtaW4iXX0.5vEcyHquQz506LBHLaqZgOP7geiKE9LIqQzfRlbadDI"
		//minerAddr = "121.204.248.35:52345"
	}

	miners, err := GetAddressFromEnv("MINERS_ID")
	if err != nil {
		log.Infof("no miner found")
	}
	wallets, err := GetAddressFromEnv("LOTUS_WALLETS")
	if err != nil {
		log.Infof("no external wallet found")
	}
	var conf = &LotusOpt{
		LotusToken:      lotusToken,
		LotusAddress:    lotusAddr,
		MinerToken:      minerToken,
		MinerAddress:    minerAddr,
		Miners:          miners,
		Wallets:         wallets,
		EnableDeadlines: true,
	}
	return conf
}

// GetAddressFromEnv from os env key get lotus address.
//
// Usage: ENV_KEY="<address>:<tag>,<address>:<tag>"
func GetAddressFromEnv(envKey string) (map[address.Address]string, error) {
	var actorAddresses = make(map[address.Address]string, 0)
	env, ok := os.LookupEnv(envKey)
	if !ok {
		return nil, errors.New("now found")
	}
	s := strings.Split(env, ",")
	for _, w := range s {
		a := strings.Split(w, ":")
		fromString, err := address.NewFromString(a[0])
		if err != nil {
			log.Warnw("decode addr", a[0], err)
			continue
		}
		if len(a) < 2 {
			actorAddresses[fromString] = "unknown"
		} else {
			actorAddresses[fromString] = a[1]
		}
	}
	if len(actorAddresses) == 0 {
		return nil, errors.New("all address invalid")
	}
	return actorAddresses, nil
}

// GetApiInfoFromEnv from os env key get lotus api info.
//
// command: lotus auth api-info --perm xxx
//
// Such as: FULLNODE_API_INFO=<token>:/ip4/<address>/tcp/<port>/http
func GetApiInfoFromEnv(envKey string) (addr, token string, err error) {
	env, ok := os.LookupEnv(envKey)
	if !ok {
		return "", "", errors.New("not found")
	}
	minerAddr := strings.Split(env, "/")
	conn, err := net.DialTimeout("tcp", minerAddr[2]+":"+minerAddr[4], time.Second*2)
	if err != nil {
		return "", "", err
	}

	defer func() {
		_ = conn.Close()
	}()
	addr = minerAddr[2] + ":" + minerAddr[4]
	token = strings.Split(env, ":")[0]
	return addr, token, nil
}

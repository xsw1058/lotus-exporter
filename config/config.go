package config

import (
	"errors"
	"flag"
	"github.com/filecoin-project/go-address"
	logging "github.com/ipfs/go-log/v2"
	"os"
	"strings"
)

var log = logging.Logger("config")

type LotusOpt struct {
	LotusToken               string
	LotusAddress             string
	MinerToken               string
	MinerAddress             string
	Miners                   map[address.Address]string
	Wallets                  map[address.Address]string
	EnableDeadlinesCollector bool
	EnableSectorCollector    bool
	SectorDetailNum          int
	SectorExpirationBuckets  int
	ListenPort               string
}

func DefaultOpt() LotusOpt {
	listen := flag.String("l", ":9002", "listen port")
	sectorDetailNum := flag.Int("n", 3, "export miner sector detail number")
	sectorExpirationBuckets := flag.Int("b", 18, "sector expiration buckets")
	enableDeadlines := flag.Bool("d", true, "enable deadline collector")
	enableSectorCollector := flag.Bool("s", true, "enable sector collector")
	flag.Parse()

	lotusAddr, lotusToken, err := GetApiInfoFromEnv("FULLNODE_API_INFO")
	if err != nil {
		log.Warnf("full node api info get err:%v", err)
	}

	minerAddr, minerToken, err := GetApiInfoFromEnv("MINER_API_INFO")
	if err != nil {
		log.Debugf("miner api info get err:%v", err)
	}

	miners, err := GetAddressFromEnv("MINERS_ID")
	if err != nil {
		log.Debugf("no miner found")
	}
	wallets, err := GetAddressFromEnv("LOTUS_WALLETS")
	if err != nil {
		log.Debugf("no external wallet found")
	}
	return LotusOpt{
		LotusToken:               lotusToken,
		LotusAddress:             lotusAddr,
		MinerToken:               minerToken,
		MinerAddress:             minerAddr,
		Miners:                   miners,
		Wallets:                  wallets,
		EnableDeadlinesCollector: *enableDeadlines,
		EnableSectorCollector:    *enableSectorCollector,
		SectorDetailNum:          *sectorDetailNum,
		SectorExpirationBuckets:  *sectorExpirationBuckets,
		ListenPort:               *listen,
	}
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
	// 发起TCP链接进行检测
	//{
	//conn, err := net.DialTimeout("tcp", minerAddr[2]+":"+minerAddr[4], time.Second*2)
	//if err != nil {
	//	return "", "", err
	//}
	//
	//defer func() {
	//	_ = conn.Close()
	//}()
	//}
	addr = minerAddr[2] + ":" + minerAddr[4]
	token = strings.Split(env, ":")[0]
	return addr, token, nil
}

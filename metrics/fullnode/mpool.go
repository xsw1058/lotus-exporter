package fullnode

import (
	"errors"
	"fmt"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/xsw1058/lotus-exporter/metrics"
	"reflect"
	"strconv"
)

type LotusMessage struct {
	*types.SignedMessage
	MethodStr string
	Tag       string
}

type MessageMetric struct {
	MessageTotal    int
	FollowsMessages []LotusMessage
}

func (l *FullNode) UpdateMessagePool() (*MessageMetric, error) {

	var t types.TipSetKey
	chainHead, err := l.api.ChainHead(l.ctx)
	if err != nil {
		t = l.chain.chainHead.Key()
	}
	t = chainHead.Key()

	messages, err := l.api.MpoolPending(l.ctx, t)
	if err != nil {
		return nil, err
	}
	var localMessages []LotusMessage
	for _, m := range messages {
		var actor *ActorBase
		actorTo, okTo := l.isAttentionActor(m.Message.To)
		actorFrom, okFrom := l.isAttentionActor(m.Message.From)

		switch {
		case okFrom:
			actor = actorFrom
		case okTo:
			actor = actorTo
		default:
			continue
		}

		toActor, err := l.api.StateGetActor(l.ctx, m.Message.To, l.chain.chainHead.Key())
		if err != nil {
			log.Warnln(err)
			continue
		}

		methodStr, err := GetMessageMethodStringByNum(l.chain.ciDs, toActor.Code, m.Message.Method)
		//methodStr, err := GetMessageMethodStringFromActorBase(actor, m.Message.Method)
		if err != nil {
			log.Debugf("convert message num to string: %v, from:%v, to:%v, err:%v", m.Cid().String(), m.Message.From, m.Message.To, err)
			methodStr = m.Message.Method.String()
		}
		localMessages = append(localMessages, LotusMessage{
			Tag:           actor.Tag,
			SignedMessage: m,
			MethodStr:     methodStr,
		})

	}
	c := MessageMetric{
		MessageTotal:    len(messages),
		FollowsMessages: localMessages,
	}
	return &c, nil
}

type MPool struct {
	MPoolCounter     *prometheus.GaugeVec
	AttentionMessage *prometheus.GaugeVec
	native           *MessageMetric
}

func NewMPoolCollector(data *MessageMetric) (metrics.LotusCollector, error) {
	return &MPool{
		MPoolCounter: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: NameSpace,
				Name:      "mpool_pending_number",
				Help:      "return local message pool number.",
			}, []string{}),
		AttentionMessage: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: NameSpace,
				Name:      "attention_message",
				Help:      "return attention actors message detail.",
			}, []string{"from", "to", "method", "cid", "tag"}),
		native: data,
	}, nil
}
func (f *MPool) Update(ch chan<- prometheus.Metric) {
	f.MPoolCounter.Reset()
	f.AttentionMessage.Reset()
	f.MPoolCounter.WithLabelValues().Set(float64(f.native.MessageTotal))
	for _, mm := range f.native.FollowsMessages {
		balance, err := strconv.ParseFloat(types.FIL(mm.Message.Value).Unitless(), 64)
		if err != nil {
			log.Warnf("failed decode message(%s) value to fil: %v", mm.Cid(), err)
			continue
		}
		f.AttentionMessage.WithLabelValues(
			mm.Message.From.String(), mm.Message.To.String(), mm.MethodStr, mm.Cid().String(), mm.Tag).Set(balance)
	}
	f.MPoolCounter.Collect(ch)
	f.AttentionMessage.Collect(ch)
}

func GetMessageMethodStringFromActorBase(actorTo *ActorBase, methodNum abi.MethodNum) (string, error) {
	const undefinedMethod = "undefined"

	switch methodNum {
	case builtin.MethodSend:
		return "Send", nil
	case builtin.MethodConstructor:
		return "Constructor", nil
	case builtin.UniversalReceiverHookMethodNum:
		return "UniversalReceiverHook", nil
	}

	var methodStruct interface{}
	var undefinedMethodError = errors.New(fmt.Sprintf("unsupport key %v", actorTo.ActorType))
	switch actorTo.ActorType {
	case actors.SystemKey:
		return undefinedMethod, undefinedMethodError
	case actors.AccountKey:
		methodStruct = builtin.MethodsAccount
	case actors.InitKey:
		methodStruct = builtin.MethodsInit
	case actors.CronKey:
		methodStruct = builtin.MethodsCron
	case actors.RewardKey:
		methodStruct = builtin.MethodsReward
	case actors.MultisigKey:
		methodStruct = builtin.MethodsMultisig
	case actors.PaychKey:
		methodStruct = builtin.MethodsPaych
	case actors.MarketKey:
		methodStruct = builtin.MethodsMarket
	case actors.PowerKey:
		methodStruct = builtin.MethodsPower
	case actors.MinerKey:
		methodStruct = builtin.MethodsMiner
	case actors.VerifregKey:
		methodStruct = builtin.MethodsVerifiedRegistry
	case actors.DatacapKey:
		methodStruct = builtin.MethodsDatacap
	default:
		return undefinedMethod, undefinedMethodError
	}
	return Uint64ValueFieldName(uint64(methodNum), methodStruct)
}

func GetMessageMethodStringByNum(actorCodeCIDs map[string]cid.Cid, actorCode cid.Cid, methodNum abi.MethodNum) (string, error) {
	const undefinedMethod = "undefined"

	switch methodNum {
	case builtin.MethodSend:
		return "Send", nil
	case builtin.MethodConstructor:
		return "Constructor", nil
	case builtin.UniversalReceiverHookMethodNum:
		return "UniversalReceiverHook", nil
	}

	var methodStruct interface{}
	var undefinedMethodError = errors.New(fmt.Sprintf("no match cid key %v", actorCode))

	for k, c := range actorCodeCIDs {
		if actorCode == c {
			switch k {
			case actors.SystemKey:
				return undefinedMethod, undefinedMethodError
			case actors.AccountKey:
				methodStruct = builtin.MethodsAccount
			case actors.InitKey:
				methodStruct = builtin.MethodsInit
			case actors.CronKey:
				methodStruct = builtin.MethodsCron
			case actors.RewardKey:
				methodStruct = builtin.MethodsReward
			case actors.MultisigKey:
				methodStruct = builtin.MethodsMultisig
			case actors.PaychKey:
				methodStruct = builtin.MethodsPaych
			case actors.MarketKey:
				methodStruct = builtin.MethodsMarket
			case actors.PowerKey:
				methodStruct = builtin.MethodsPower
			case actors.MinerKey:
				methodStruct = builtin.MethodsMiner
			case actors.VerifregKey:
				methodStruct = builtin.MethodsVerifiedRegistry
			case actors.DatacapKey:
				methodStruct = builtin.MethodsDatacap
			}
			return Uint64ValueFieldName(uint64(methodNum), methodStruct)
		}
	}
	return undefinedMethod, undefinedMethodError
}

// Uint64ValueFieldName 从一个结构体中取出值对应的字段名
func Uint64ValueFieldName(value uint64, someStruct interface{}) (string, error) {
	const unknownMethod = "unknown"
	t := reflect.TypeOf(someStruct)
	va := reflect.ValueOf(someStruct)

	if t.Kind() != reflect.Struct {
		return unknownMethod, errors.New(fmt.Sprintf("can not get field name from '%v' type", t))
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		v := va.Field(i)
		// 1. 判断字段类型为uint64
		if field.Type.Kind() == reflect.Uint64 {
			// 2. 判断字段值是否与给定的值相同
			if v.Uint() == value {
				return field.Name, nil
			}
		}
	}

	return unknownMethod, errors.New(fmt.Sprintf("'%v' not matched field name", value))
}

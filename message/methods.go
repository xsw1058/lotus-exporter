package message

import (
	"errors"
	"fmt"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/specs-actors/v8/actors/builtin"
	"github.com/ipfs/go-cid"
	"reflect"
)

const unknownMethod = "Unknown"

func GetMessageMethodStringByNum(actorCodeCIDs map[string]cid.Cid, actorCodeStr string, methodNum abi.MethodNum) (string, error) {
	if methodNum == builtin.MethodSend {
		return "Send", nil
	}

	var noMethodErr = errors.New(fmt.Sprintf("%v not have method", actorCodeStr))

	for k, c := range actorCodeCIDs {
		var methodStruct interface{}
		if actorCodeStr == c.String() {
			switch k {
			case "storagepower":
				methodStruct = builtin.MethodsPower
			case "system":
				return unknownMethod, noMethodErr
			case "account":
				methodStruct = builtin.MethodsAccount
			case "cron":
				methodStruct = builtin.MethodsCron
			case "init":
				methodStruct = builtin.MethodsInit
			case "reward":
				methodStruct = builtin.MethodsReward
			case "storagemarket":
				methodStruct = builtin.MethodsMarket
			case "storageminer":
				methodStruct = builtin.MethodsMiner
			case "verifiedregistry":
				methodStruct = builtin.MethodsVerifiedRegistry
			case "_manifest":
				return unknownMethod, noMethodErr
			case "multisig":
				methodStruct = builtin.MethodsMultisig
			case "paymentchannel":
				methodStruct = builtin.MethodsPaych
			default:
				return unknownMethod, noMethodErr
			}
			return Uint64ValueFieldName(uint64(methodNum), methodStruct)
		}
	}
	return unknownMethod, noMethodErr
}

// Uint64ValueFieldName 从一个结构体中取出值对应的字段名
func Uint64ValueFieldName(value uint64, someStruct interface{}) (string, error) {
	t := reflect.TypeOf(someStruct)
	v := reflect.ValueOf(someStruct)

	if t.Kind() != reflect.Struct {
		return unknownMethod, errors.New(fmt.Sprintf("can not get field name from '%v' type", t))
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		v := v.Field(i)
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

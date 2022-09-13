//go:build linux || darwin

package message

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors"
	"reflect"
	"testing"
)

func TestFieldName(t *testing.T) {
	type test struct {
		inputNumber uint64
		inputStruct interface{}
		want        string
	}

	var tests = map[string]test{
		"stringTest": {inputNumber: 12, inputStruct: "12345", want: unknownMethod},
		"sliceTest":  {inputNumber: 12, inputStruct: []uint64{56, 12, 34}, want: unknownMethod},
		"structTest1": {inputNumber: 12, inputStruct: struct {
			Name string
			Age  uint64
		}{"CCC", 122}, want: unknownMethod},
		"structTest2": {inputNumber: 12, inputStruct: struct {
			Name string
			Age  uint64
		}{"CCC", 12}, want: "Age"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, _ := Uint64ValueFieldName(tc.inputNumber, tc.inputStruct)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("expected:%v, got:%v", tc.want, got)
			}
		})
	}
}

func TestMethodString(t *testing.T) {

	iDs, err := actors.GetActorCodeIDs(8)
	if err != nil {
		t.Fatal(err)
	}
	type test struct {
		want    string
		idCode  string
		methodN uint64
	}

	tests := map[string]test{
		"storageMiner_0": {
			want:    "Send",
			idCode:  "bafk2bzacecgnynvd3tene3bvqoknuspit56canij5bpra6wl4mrq2mxxwriyu",
			methodN: 0,
		},
		"storageMiner_27": {
			want:    "ProveReplicaUpdates",
			idCode:  "bafk2bzacecgnynvd3tene3bvqoknuspit56canij5bpra6wl4mrq2mxxwriyu",
			methodN: 27,
		},
		"cron": {
			want:    "EpochTick",
			idCode:  "bafk2bzacecqb3eolfurehny6yp7tgmapib4ocazo5ilkopjce2c7wc2bcec62",
			methodN: 2,
		},
		"init": {
			want:    "Exec",
			idCode:  "bafk2bzaceaipvjhoxmtofsnv3aj6gj5ida4afdrxa4ewku2hfipdlxpaektlw",
			methodN: 2,
		},
		"multisig": {
			want:    "LockBalance",
			idCode:  "bafk2bzacebhldfjuy4o5v7amrhp5p2gzv2qo5275jut4adnbyp56fxkwy5fag",
			methodN: 9,
		},
		"reward": {
			want:    "UpdateNetworkKPI",
			idCode:  "bafk2bzacecwzzxlgjiavnc3545cqqil3cmq4hgpvfp2crguxy2pl5ybusfsbe",
			methodN: 4,
		},
		"account": {
			want:    "PubkeyAddress",
			idCode:  "bafk2bzacedudbf7fc5va57t3tmo63snmt3en4iaidv4vo3qlyacbxaa6hlx6y",
			methodN: 2,
		},
		"paymentchannel": {
			want:    "Collect",
			idCode:  "bafk2bzacebalad3f72wyk7qyilvfjijcwubdspytnyzlrhvn73254gqis44rq",
			methodN: 4,
		},
		"storagemarket": {
			want:    "CronTick",
			idCode:  "bafk2bzacediohrxkp2fbsl4yj4jlupjdkgsiwqb4zuezvinhdo2j5hrxco62q",
			methodN: 9,
		},
		"storagepower": {
			want:    "CurrentTotalPower",
			idCode:  "bafk2bzacebjvqva6ppvysn5xpmiqcdfelwbbcxmghx5ww6hr37cgred6dyrpm",
			methodN: 9,
		},
		"other": {
			want:    unknownMethod,
			idCode:  "abcdefght",
			methodN: 98,
		},
	}

	for name, te := range tests {
		t.Run(name, func(t *testing.T) {
			got, _ := GetMessageMethodStringByNum(iDs, te.idCode, abi.MethodNum(te.methodN))
			if !reflect.DeepEqual(got, te.want) {
				t.Errorf("expected:%v, got:%v", te.want, got)
			}
		})
	}

}

package metrics

const NameSpace = "lotus"
const InfoKey = "info"
const Demo1Key = "demo1"
const Demo2Key = "demo2"
const Demo3Key = "demo3"

var (
	RightNowUpdate     = []string{"right_now", Demo1Key}
	EachEpochUpdate    = []string{"each_epoch", Demo2Key}
	EachDeadlineUpdate = []string{"each_deadline", Demo3Key}
)

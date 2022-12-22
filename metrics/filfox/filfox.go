package filfox

import (
	"encoding/json"
	"errors"
	"fmt"
	logging "github.com/ipfs/go-log/v2"
	"net/http"
	"strconv"
	"time"
)

var (
	log = logging.Logger("filfox")
)

const (
	Burn     = "burn"
	Reward   = "reward"
	Transfer = "transfer"
	Send     = "send"
	Receive  = "receive"
)

type FilFox struct {
	URLPrefix string
	Client    http.Client
}

func NewFilFox() *FilFox {
	return &FilFox{
		URLPrefix: "https://filfox.info/api/v1",
		Client:    http.Client{},
	}
}

type TransferResult struct {
	TotalCount int64            `json:"totalCount,omitempty"`
	Transfers  []TransferFilFox `json:"transfers,omitempty"`
	Types      []string         `json:"types,omitempty"`
}

type TransferFilFox struct {
	From      string `json:"from,omitempty"`
	Height    int64  `json:"height,omitempty"`
	Message   string `json:"message,omitempty"`
	Timestamp int64  `json:"timestamp,omitempty"`
	To        string `json:"to,omitempty"`
	Type      string `json:"type,omitempty"`
	Value     string `json:"value,omitempty"`
}

type Transfers []TransferFilFox

func (t Transfers) Len() int {
	return len(t)
}

func (t Transfers) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
}

func (t Transfers) Less(i, j int) bool {
	return t[i].Timestamp < t[j].Timestamp
}

func (t Transfers) RemoveDuplicate() Transfers {
	resArr := make([]TransferFilFox, 0)
	tmpMap := make(map[TransferFilFox]interface{})

	for _, value := range t {

		if _, ok := tmpMap[value]; !ok {
			resArr = append(resArr, value)
			tmpMap[value] = nil
		}
	}
	return resArr
}

func (s *TransferFilFox) Equal(detail TransferFilFox) bool {
	return s.To == detail.To &&
		s.From == detail.From &&
		s.Timestamp == detail.Timestamp &&
		s.Type == detail.Type &&
		s.Value == detail.Value &&
		s.Message == detail.Message
}

type SummaryDetail struct {
	// 表示统计的开始时间和结束时间
	StartAt time.Time
	EndAt   time.Time

	// map[string]float64 的key 表示一个地址, float64为值,单位为FIL.
	Burn     map[PeerActor]float64
	Reward   map[PeerActor]float64
	Transfer TransferSummary
}

type SummaryDetails struct {
	Details []SummaryDetail
}

func (s *SummaryDetails) EarliestStartAt() time.Time {
	var earliestStartAt = time.Now()

	for _, detail := range s.Details {
		if detail.StartAt.Before(earliestStartAt) {
			earliestStartAt = detail.StartAt
		}
	}

	return earliestStartAt
}

type PeerActor struct {
	Address string
	Tag     string
}

type TransferSummary struct {
	Send    map[PeerActor]float64
	Receive map[PeerActor]float64
}

func NewSummaryDetailFromStartString(startAt string) (*SummaryDetail, error) {
	sAt, err := time.Parse("2006-01-02", startAt)
	if err != nil {
		return nil, err
	}
	eAt := sAt.Add(time.Hour * 24)
	return &SummaryDetail{
		StartAt: sAt,
		EndAt:   eAt,
		//Durations: time.Hour * 24,
		Burn:   make(map[PeerActor]float64),
		Reward: make(map[PeerActor]float64),
		Transfer: TransferSummary{
			Send:    make(map[PeerActor]float64),
			Receive: make(map[PeerActor]float64),
		},
	}, nil
}

func NewSummaryDetails() SummaryDetails {
	now := time.Now()
	var summaryDetails []SummaryDetail

	todayStartAt := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)

	yesterdayStartAt := todayStartAt.AddDate(0, 0, -1)

	thisMonthStartAt := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)

	lastMonthStartAt := thisMonthStartAt.AddDate(0, -1, 0)

	today := SummaryDetail{
		StartAt: todayStartAt,
		EndAt:   todayStartAt.AddDate(0, 0, 1),
		Burn:    make(map[PeerActor]float64),
		Reward:  make(map[PeerActor]float64),
		Transfer: TransferSummary{
			Send:    make(map[PeerActor]float64),
			Receive: make(map[PeerActor]float64),
		},
	}

	yesterday := SummaryDetail{
		StartAt: yesterdayStartAt,
		EndAt:   yesterdayStartAt.AddDate(0, 0, 1),
		Burn:    make(map[PeerActor]float64),
		Reward:  make(map[PeerActor]float64),
		Transfer: TransferSummary{
			Send:    make(map[PeerActor]float64),
			Receive: make(map[PeerActor]float64),
		},
	}

	thisMonth := SummaryDetail{
		StartAt: thisMonthStartAt,
		EndAt:   thisMonthStartAt.AddDate(0, 1, 0),
		Burn:    make(map[PeerActor]float64),
		Reward:  make(map[PeerActor]float64),
		Transfer: TransferSummary{
			Send:    make(map[PeerActor]float64),
			Receive: make(map[PeerActor]float64),
		},
	}

	lastMonth := SummaryDetail{
		StartAt: lastMonthStartAt,
		EndAt:   lastMonthStartAt.AddDate(0, 1, 0),
		Burn:    make(map[PeerActor]float64),
		Reward:  make(map[PeerActor]float64),
		Transfer: TransferSummary{
			Send:    make(map[PeerActor]float64),
			Receive: make(map[PeerActor]float64),
		},
	}

	summaryDetails = append(summaryDetails, today, yesterday, thisMonth, lastMonth)

	return SummaryDetails{Details: summaryDetails}
}

func (s *SummaryDetail) Between(at time.Time) bool {
	return at.After(s.StartAt) && at.Before(s.EndAt)
}

func (s *SummaryDetail) String() string {
	now := time.Now()

	todayStartAt := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	todayEndAt := todayStartAt.AddDate(0, 0, 1)

	yesterdayStartAt := todayStartAt.AddDate(0, 0, -1)
	yesterdayEndAt := yesterdayStartAt.AddDate(0, 0, 1)

	thisMonthStartAt := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
	thisMonthEndAt := thisMonthStartAt.AddDate(0, 1, 0)

	lastMonthStartAt := thisMonthStartAt.AddDate(0, -1, 0)
	lastMonthEndAt := lastMonthStartAt.AddDate(0, 1, 0)

	switch {
	case s.StartAt.Unix() == todayStartAt.Unix() && s.EndAt.Unix() == todayEndAt.Unix():
		return "today"
	case s.StartAt.Unix() == yesterdayStartAt.Unix() && s.EndAt.Unix() == yesterdayEndAt.Unix():
		return "yesterday"
	case s.StartAt.Unix() == thisMonthStartAt.Unix() && s.EndAt.Unix() == thisMonthEndAt.Unix():
		return "this_month"
	case s.StartAt.Unix() == lastMonthStartAt.Unix() && s.EndAt.Unix() == lastMonthEndAt.Unix():
		return "last_month"
	default:
		return s.StartAt.Local().Format("2006-01-02 15:04:05") + "~" + s.EndAt.Local().Format("2006-01-02 15:04:05")
	}

}

func (f *FilFox) getTransferOnce(address string, pageSize, page int) (Transfers, int64, error) {

	urlStr := fmt.Sprintf("https://filfox.info/api/v1/address/%s/transfers", address)

	request, err := http.NewRequest(http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, 0, err
	}

	q := request.URL.Query()
	q.Add("pageSize", strconv.Itoa(pageSize))
	q.Add("page", strconv.Itoa(page))
	q.Add("type", "transfer")

	request.URL.RawQuery = q.Encode()
	request.Header.Add("Accept", "application/json, text/plain, */*")
	request.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/108.0.0.0 Safari/537.36 Edg/108.0.1462.54")

	resp, err := f.Client.Do(request)
	if err != nil {
		return nil, 0, err
	}

	var res TransferResult
	err = json.NewDecoder(resp.Body).Decode(&res)
	if err != nil {
		return nil, 0, err
	}

	if len(res.Transfers) == 0 || res.TotalCount == 0 {
		return nil, 0, errors.New(fmt.Sprintf("not transfer response. url: %v code: %v status: %v", request.URL.String(), resp.StatusCode, resp.Status))
	}
	return res.Transfers, res.TotalCount, nil
}

func (f *FilFox) GetLatestPageTransfer(address string) (Transfers, int64, error) {
	return f.getTransferOnce(address, 20, 0)
}

func (f *FilFox) GetTransferSince(address string, since time.Time) (Transfers, int64, error) {
start:
	var TransferResults Transfers
	var num = 0
	var try = 0
	var totalCount int64
	for {
		if try > 1 {
			break
		}
		result, count, err := f.getTransferOnce(address, 20, num)
		if err != nil {
			log.Warnw("request failed", "err", err)
			time.Sleep(time.Second * 3)
			try++
			continue
		}

		if totalCount == 0 {
			totalCount = count
		}

		if totalCount != count {
			log.Infow("may be new transfer coming")
			goto start
		}

		e := result[len(result)-1]

		TransferResults = append(TransferResults, result...)

		if e.Timestamp < since.Unix() {
			break
		}

		num++

		time.Sleep(time.Millisecond * 500)
	}
	if len(TransferResults) == 0 {
		return nil, 0, errors.New(fmt.Sprintf("not transfer. address: %v since: %v", address, since))
	}

	return TransferResults, totalCount, nil
}

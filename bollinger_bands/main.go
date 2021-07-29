package bollinger_bands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/pkg/errors"
)

type StockRespBody struct {
	Statuscode int       `json:"statusCode"`
	Message    string    `json:"message"`
	Data       StockData `json:"data"`
}

type StockData struct {
	S        string      `json:"s"`
	T        []int       `json:"t"`
	O        []float64   `json:"o"`
	H        []float64   `json:"h"`
	L        []float64   `json:"l"`
	C        []float64   `json:"c"`
	V        []float64   `json:"v"`
	Vwap     []float64   `json:"vwap"`
	Quote    interface{} `json:"quote"`
	Session  [][]int     `json:"session"`
	Nexttime interface{} `json:"nextTime"`
}

const (
	malen = 20

	nearParam = 2 * 0.01 * 0.01
)

var (
	listUrl = "https://www.cnyes.com/twstock/stock_astock.aspx"
	baseUrl = "https://www.cnyes.com/twstock/"
	apiUrl  = "https://ws.api.cnyes.com/ws/api/v1/charting/history?resolution=D&symbol=TWS:%d:STOCK&from=%d&to=%d"
)

func Run() []int {
	var wg sync.WaitGroup
	done := make(chan bool)
	targetChan := make(chan int)

	var targets []int
	wg.Add(1)
	go func() {
		defer wg.Done()
		targets = getTargetList(targetChan, done)
	}()

	groupList := getGroupList()
	findTickersByGroup(groupList, targetChan, done)

	wg.Wait()
	return targets
}

func getTargetList(targetChan <-chan int, done <-chan bool) []int {
	var result []int
	for {
		select {
		case t := <-targetChan:
			result = append(result, t)
		case <-done:
			return result
		}
	}
}

func getGroupList() map[string]string {
	b, err := request(listUrl)
	if err != nil {
		log.Fatal("get group list err")
	}

	r := bytes.NewReader(b)

	// Load the HTML document
	dom, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		log.Fatal(err)
	}

	data := make(map[string]string)
	// Find the link items
	dom.Find("#kinditem_0").Find("a").Each(func(i int, s *goquery.Selection) {
		// For each item found, get the band and title
		title := s.Text()
		url, _ := s.Attr("href")
		data[title] = url
	})

	return data
}

func findTickersByGroup(groupList map[string]string, targetChan chan<- int, done chan<- bool) {
	var wg sync.WaitGroup
	wg.Add(len(groupList))

	for _, u := range groupList {
		u := u
		go func() {
			defer wg.Done()
			stocks := getStocks(u)
			findTargetTickers(stocks, targetChan)
		}()
	}

	wg.Wait()
	close(done)
}

func getStocks(u string) []int {
	url := baseUrl + u
	b, err := request(url)
	if err != nil {
		log.Fatal("get stock list err")
	}

	r := bytes.NewReader(b)

	// Load the HTML document
	dom, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		log.Fatal(err)
	}

	var data []int
	// Find the link items
	dom.Find(".TableBox").Find("a").Each(func(i int, s *goquery.Selection) {
		// For each item found, get the band and title
		title := s.Text()

		num, err := strconv.Atoi(title)
		if err == nil {
			data = append(data, num)
		}
	})

	return data
}

func findTargetTickers(tickers []int, targetChan chan<- int) {
	from := time.Now().Unix()
	to := time.Now().AddDate(0, 0, -30).Unix()

	for _, i := range tickers {
		url := fmt.Sprintf(apiUrl, i, from, to)
		res, err := request(url)
		if err != nil {
			log.Printf("[%d] request failed\n", i)
			continue
		}

		var stockBody StockRespBody
		if err := json.Unmarshal(res, &stockBody); err != nil {
			log.Printf("[%d] json unmarshal failed, from:%v, to:%v\n", i, from, to)
			continue
		}

		if len(stockBody.Data.C) < malen {
			continue
		}

		if calVol(stockBody.Data.V, 5) < 1000 {
			continue
		}

		if k, d := calKD(stockBody.Data); k > 25 || d > 25 {
			continue
		}

		todayClose := stockBody.Data.C[0]
		closes := stockBody.Data.C[:malen]
		bbL, _ := calBB(closes)

		h := bbL + nearParam*bbL

		if todayClose <= h {
			targetChan <- i
		}
	}
}

func calVol(data []float64, day int) float64 {
	var tmp float64
	for _, v := range data[:day] {
		tmp += v
	}

	return tmp / float64(day)
}

func calKD(data StockData) (k float64, d float64) {
	k = float64(60)
	d = float64(60)

	for i := 9; i > 0; i-- {
		//		log.Printf("====== [%d] =====\n", i)
		low := data.L[i : i+9]
		high := data.H[i : i+9]
		l := min(low)
		h := max(high)
		c := data.C[i]

		rsv := (c - l) / (h - l) * 100
		//		log.Printf("low_range: %v, low: %f\n", low, l)
		//		log.Printf("high_range: %v, high: %f\n", high, h)
		//		log.Printf("close: %f, rsv: %f", c, rsv)

		k = rsv/3 + float64(2)/float64(3)*k
		//		log.Println("k =", k)

		d = k/3 + float64(2)/float64(3)*d
		//		log.Println("d =", d)
	}

	return
}

func min(values []float64) float64 {
	min := values[0]
	for _, v := range values {
		if v < min {
			min = v
		}
	}

	return min
}

func max(values []float64) float64 {
	max := values[0]
	for _, v := range values {
		if v > max {
			max = v
		}
	}

	return max
}

func calBB(data []float64) (float64, float64) {
	var sum float64
	for _, c := range data {
		sum += c
	}
	ma := sum / float64(malen)

	var variance float64
	for _, v := range data {
		variance += math.Pow(v-ma, 2)
	}

	sigma := math.Sqrt(variance / malen)
	return ma - 2*sigma, ma + 2*sigma
}

func request(url string) ([]byte, error) {
	var respBody []byte
	var err error
	resp, respErr := http.Get(url)

	if respErr != nil || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		// if we got a valid http response, try to read body so we can reuse the connection
		// see https://golang.org/pkg/net/http/#Client.Do
		if respErr == nil {
			respBody, err = ioutil.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				return nil, errors.Wrap(err, "read response failed")
			}
		}
	} else {
		respBody, err = ioutil.ReadAll(resp.Body)
		defer resp.Body.Close()
		if err != nil {
			return nil, errors.Wrap(err, "read response failed")
		}
	}

	return respBody, nil
}

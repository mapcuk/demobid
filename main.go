package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

const serverAddr = "0:8080"
const MaxDSP = 3

func main() {
	rand.Seed(time.Now().UnixNano())

	router := newRouter()
	s := &http.Server{
		Addr:         serverAddr,
		Handler:      router,
		ReadTimeout:  100 * time.Millisecond,
		WriteTimeout: 100 * time.Millisecond,
	}
	log.Printf("starting server %s", s.Addr)
	log.Fatal(s.ListenAndServe())
}

func newRouter() http.Handler {
	router := chi.NewRouter()
	router.Get("/bid", HandlerBid)
	router.Get("/auction", HandlerAuction)
	return router
}

type Resp struct {
	Price float64 `json:"price"`
}

// HandlerBid expects 2 params:
// p - float
// dsp - uInt [1:3]
// responds with JSON like {price:10.1}
func HandlerBid(w http.ResponseWriter, r *http.Request) {
	vars := r.URL.Query()

	dsp, err := strconv.ParseUint(vars.Get("dsp"), 10, 32)
	if err != nil || dsp > MaxDSP || dsp < 1 {
		http.Error(w, "bad dsp parameter", http.StatusBadRequest)
		return
	}

	resp := Resp{}
	if floor, err := strconv.ParseFloat(vars.Get("p"), 64); err == nil {
		resp.Price = floor + rand.Float64()*100
		resp.Price = math.Round(resp.Price*100) / 100
	} else {
		http.Error(w, "bad p parameter", http.StatusBadRequest)
		return
	}

	// NOTICE: sleep 10 - 100 ms
	delayTimeMs := time.Duration(10 * (rand.Intn(9) + 1))
	time.Sleep(delayTimeMs * time.Millisecond)

	body, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	if _, err = w.Write(body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

type DspResult struct {
	DSPId    int
	BidPrice float64
}
type DspResults []DspResult

func (b DspResults) Len() int           { return len(b) }
func (b DspResults) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b DspResults) Less(i, j int) bool { return b[i].BidPrice < b[j].BidPrice }

func HandlerAuction(w http.ResponseWriter, r *http.Request) {
	client := http.Client{
		Timeout: 100 * time.Millisecond,
	}
	// NOTICE: generate random floor price
	floor := rand.Float64() * 10

	dspResults := DspResults{}
	queue := make(chan DspResult, 1)

	allDone := make(chan struct{}, 1)
	go func() {
		for dspRes := range queue {
			dspResults = append(dspResults, dspRes)
		}
		log.Println("finished asking DSPs")
		allDone <- struct{}{}
	}()

	wgDSP := sync.WaitGroup{}
	for dspId := 1; dspId < MaxDSP+1; dspId++ {
		wgDSP.Add(1)
		go func(innerDSPId int) {
			err := askDSP(&wgDSP, &client, queue, floor, innerDSPId)
			if err != nil {
				log.Printf("error %s during processing DSP %d", err, innerDSPId)
			}
		}(dspId)
	}
	wgDSP.Wait()
	close(queue)
	<-allDone
	log.Printf("Got %d results", len(dspResults))
	for _, k := range dspResults {
		log.Printf("DSP %d bid price %g", k.DSPId, k.BidPrice)
	}
	sort.Sort(dspResults)
	winner := dspResults[len(dspResults)-1]
	log.Printf("Highest bid %g from DSP %d", winner.BidPrice, winner.DSPId)
}

func askDSP(wg *sync.WaitGroup, client *http.Client, qDSPResults chan DspResult, floor float64, dspId int) error {
	defer wg.Done()
	log.Printf("asking DSP %d", dspId)
	bidURL := makeBidURL(floor, dspId)
	bidResp, err := client.Get(bidURL)
	if err != nil {
		return err
	}
	bidRespBytes, _ := ioutil.ReadAll(bidResp.Body)
	resp := Resp{}
	err = json.Unmarshal(bidRespBytes, &resp)
	if err != nil {
		return err
	}
	qDSPResults <- DspResult{DSPId: dspId, BidPrice: resp.Price}
	return nil
}

func makeBidURL(floor float64, dspId int) string {
	params := url.Values{}
	params.Add("p", strconv.FormatFloat(floor, 'f', 3, 64))
	params.Add("dsp", strconv.Itoa(dspId))

	addr := url.URL{
		Scheme:   "http",
		Host:     serverAddr,
		Path:     "/bid",
		RawQuery: params.Encode(),
	}
	return addr.String()
}

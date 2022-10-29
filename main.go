package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

const serverAddr = "0:8080"

func main() {
	rand.Seed(time.Now().UnixNano())

	router := newRouter()
	s := &http.Server{
		Addr:         serverAddr,
		Handler:      router,
		ReadTimeout:  100 * time.Millisecond,
		WriteTimeout: 100 * time.Millisecond,
	}
	log.Printf("starting server %s\n", s.Addr)
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
	if err != nil || dsp > 3 || dsp < 1 {
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

func HandlerAuction(w http.ResponseWriter, r *http.Request) {
	client := http.Client{
		Timeout: 100 * time.Millisecond,
	}
	wg := sync.WaitGroup{}
	for dspId := 1; dspId < 4; dspId++ {
		wg.Add(1)
		go askDSP(&wg, &client, dspId)
	}
	wg.Wait()
}

func askDSP(wg *sync.WaitGroup, client *http.Client, dspId int) {
	defer wg.Done()
	// NOTICE: generate random floor price
	floor := rand.Float64() * 10
	bidURL := makeBidURL(floor, dspId)
	resp, err := client.Get(bidURL)
	if err != nil {
		log.Println(err)
		return
	}
	bidResp, _ := ioutil.ReadAll(resp.Body)
	log.Println(string(bidResp))
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

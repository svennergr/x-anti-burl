package main

import (
	"bufio"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

var (
	client *http.Client
	wg     sync.WaitGroup

	requestMethod string
	userAgent     string

	concurrency = 50
	maxSize     = int64(1024000)
)

func main() {
	flag.StringVar(&requestMethod, "X", "HEAD", "The HTTP Request method. For the fastest, use HEAD.")
	flag.StringVar(&userAgent, "-user-agent", "Mozilla", "The HTTP User Agent.")
	flag.Parse()

	var input io.Reader
	input = os.Stdin

	if flag.NArg() > 0 {
		file, err := os.Open(flag.Arg(0))
		if err != nil {
			fmt.Printf("failed to open file: %s\n", err)
			os.Exit(1)
		}
		input = file
	}

	sc := bufio.NewScanner(input)

	client = &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        concurrency,
			MaxIdleConnsPerHost: concurrency,
			MaxConnsPerHost:     concurrency,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
		Timeout: 5 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	semaphore := make(chan bool, concurrency)

	for sc.Scan() {
		raw := sc.Text()
		wg.Add(1)
		semaphore <- true
		go func(raw string) {
			defer wg.Done()
			u, err := url.ParseRequestURI(raw)
			if err != nil {
				return
			}
			resp, ws, err := fetchURL(u)
			if err != nil {
				return
			}
			if resp.StatusCode <= 300 || resp.StatusCode >= 500 {
				fmt.Printf("%d\t%d\t%d\t%s\t%s\n", resp.StatusCode, resp.ContentLength, ws, resp.Header.Get("Content-Type"), u.String())
			}
		}(raw)
		<-semaphore
	}

	wg.Wait()

	if sc.Err() != nil {
		fmt.Printf("error: %s\n", sc.Err())
	}
}

func fetchURL(u *url.URL) (*http.Response, int, error) {
	wordsSize := 0

	req, err := http.NewRequest(requestMethod, u.String(), nil)
	if err != nil {
		return nil, 0, err
	}

	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}

	defer resp.Body.Close()

	if resp.ContentLength <= maxSize {
		if respbody, err := ioutil.ReadAll(resp.Body); err == nil {
			resp.ContentLength = int64(utf8.RuneCountInString(string(respbody)))
			wordsSize = len(strings.Split(string(respbody), " "))
		}
	}

	io.Copy(ioutil.Discard, resp.Body)

	return resp, wordsSize, err
}

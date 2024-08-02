package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/go-resty/resty/v2"
	"github.com/patrickmn/go-cache"
)

const url = "http://localhost:8081/v1/users/"

type ResponseAPI struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Success"`
	Data    any    `json:"data,omitempty"`
}

type Logger interface {
	Errorf(format string, v ...interface{})
	Warnf(format string, v ...interface{})
	Debugf(format string, v ...interface{})
}

func createLogger() *logger {
	l := &logger{l: log.New(os.Stderr, "", log.Ldate|log.Lmicroseconds)}
	return l
}

var _ Logger = (*logger)(nil)

type logger struct {
	l *log.Logger
}

func (l *logger) Errorf(format string, v ...interface{}) {
	l.output("ERROR RESTY "+format, v...)
}

func (l *logger) Warnf(format string, v ...interface{}) {
	l.output("WARN RESTY "+format, v...)
}

func (l *logger) Debugf(format string, v ...interface{}) {
	l.output("DEBUG RESTY "+format, v...)
}

func (l *logger) output(format string, v ...interface{}) {
	if len(v) == 0 {
		l.l.Print(format)
		return
	}
	l.l.Printf(format, v...)
}

func printBackoffValues(bo *backoff.ExponentialBackOff) {
	fmt.Printf("InitialInterval: %v\n", bo.InitialInterval)
	fmt.Printf("RandomizationFactor: %v\n", bo.RandomizationFactor)
	fmt.Printf("Multiplier: %v\n", bo.Multiplier)
	fmt.Printf("MaxInterval: %v\n", bo.MaxInterval)
	fmt.Printf("MaxElapsedTime: %v\n", bo.MaxElapsedTime)
	fmt.Printf("CurrentInterval: %v\n", bo.GetElapsedTime())
	fmt.Printf("Clock: %v\n", bo.Clock)

}
func main() {
	header := map[string]string{
		"Authorization": "Bearer your_token",
		"User-Agent":    "MyAPI/1.0",
		"Content-Type":  "application/json",
		//"Transaction-Key": "123456789",
	}

	payload := map[string]interface{}{
		"email":        "3246g11test288911@gmail.com",
		"password":     "fghjklhjgf",
		"name":         "sawerr",
		"check":        10,
		"created_time": time.Now().Format("15:04"),
	}

	var logBuf bytes.Buffer
	logger := createLogger()
	logger.l.SetOutput(&logBuf)

	// Create a new resty client with an overall timeout
	client := resty.New().
		SetTimeout(5 * time.Second) // Overall client timeout
		//SetRetryCount(3).                     // Number of retry attempts
		//SetRetryWaitTime(2 * time.Second).    // Wait time between retries
		//SetRetryMaxWaitTime(10 * time.Second) // Maximum wait time for retries

	// client.AddRetryCondition(
	// 	func(r *resty.Response, err error) bool {
	// 		return err != nil || r.StatusCode() >= 500
	// 	},
	// )

	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = 1 * time.Second
	bo.MaxElapsedTime = 5 * time.Second
	bo.MaxInterval = 5 * time.Second
	bo.Multiplier = 2
	bo.RandomizationFactor = 0.5
	bo.GetElapsedTime()
	bo.Clock = backoff.SystemClock
	bo.NextBackOff()
	bo.Reset()
	printBackoffValues(bo) // Print backoff values before starting
	notify := func(err error, duration time.Duration) {
		logger.Warnf("Error: %v, backing off for %s", err, duration)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cookies := []*http.Cookie{
		{Name: "session_id", Value: "abc123", Path: "/"},
		{Name: "user_token", Value: "xyz789", Path: "/"},
	}
	logger.Debugf("cookies: %v", cookies)
	var response *resty.Response
	attempt := 0
	operation := func() error {
		attempt++

		log.Println("attempting operation", attempt)
		resp, err := client.R().
			SetBody(payload).
			SetHeaders(header).
			SetLogger(logger).   // Set the custom logger
			SetCookies(cookies). // Set the cookies
			SetContext(ctx).
			SetResult(ResponseAPI{}).
			SetDebug(true).
			Post(url)
		response = resp
		if err != nil {
			logger.Errorf("Request failed: %v", err)
		} else if response.StatusCode() >= 500 {
			err = fmt.Errorf("server error: %v", response.Status())
		}
		return err
	}

	err := backoff.RetryNotify(operation, bo, notify)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			logger.Errorf("Context deadline exceeded: %v", ctx.Err())
		} else {
			logger.Errorf("Final error: %v", err)
		}
		fmt.Printf("Error: %v\n", err)
		return
	}

	c := cache.New(5*time.Minute, 10*time.Minute)
	if cachedResponse, found := c.Get(url); found {
		fmt.Println("Using cached response")
		fmt.Printf("Response: %v\n", cachedResponse)
		return
	}

	fmt.Println("Log Output:")
	fmt.Println(logBuf.String())

	if response != nil {
		trace := response.Request.TraceInfo()
		logger.Debugf("Trace Information:\n%+v", trace)
	}

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Response: %v\n", response)

	if response.IsSuccess() {
		c.Set(url, ResponseAPI{}, cache.DefaultExpiration)
		fmt.Printf("Response Status Code: %v\n", response.StatusCode())
		fmt.Printf("Response Body: %s\n", response.String())
	} else {
		fmt.Printf("Error Response Status Code: %v\n", response.StatusCode())
		fmt.Printf("Error Response Body: %s\n", response.String())
		fmt.Printf("Error Content-Type Header: %v\n", response.Header().Get("Content-Type"))
	}
}

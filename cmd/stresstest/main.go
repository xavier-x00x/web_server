package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	url := flag.String("url", "http://localhost:80/index.php", "Target URL")
	concurrency := flag.Int("c", 100, "Number of concurrent workers")
	duration := flag.Duration("d", 10*time.Second, "Duration of the test")

	flag.Parse()

	fmt.Printf("==========================================================\n")
	fmt.Printf(" GopherStack Enterprise - Stress Test\n")
	fmt.Printf("==========================================================\n")
	fmt.Printf("Target:      %s\n", *url)
	fmt.Printf("Concurrency: %d workers\n", *concurrency)
	fmt.Printf("Duration:    %v\n", *duration)
	fmt.Printf("==========================================================\n\n")

	fmt.Printf("Starting test... Please wait %v...\n\n", *duration)

	var (
		totalRequests uint64
		successCount  uint64
		errorCount    uint64
		wg            sync.WaitGroup
	)

	// Create a transport with a larger connection pool
	tr := &http.Transport{
		MaxIdleConns:        *concurrency,
		MaxIdleConnsPerHost: *concurrency,
		IdleConnTimeout:     30 * time.Second,
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   5 * time.Second,
	}

	startTime := time.Now()
	endTime := startTime.Add(*duration)

	wg.Add(*concurrency)
	for i := 0; i < *concurrency; i++ {
		go func() {
			defer wg.Done()
			for time.Now().Before(endTime) {
				atomic.AddUint64(&totalRequests, 1)

				resp, err := client.Get(*url)
				if err != nil {
					atomic.AddUint64(&errorCount, 1)
					continue
				}

				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()

				if resp.StatusCode >= 200 && resp.StatusCode < 400 {
					atomic.AddUint64(&successCount, 1)
				} else {
					atomic.AddUint64(&errorCount, 1)
				}
			}
		}()
	}

	wg.Wait()
	actualDuration := time.Since(startTime)

	reqTotal := atomic.LoadUint64(&totalRequests)
	reqSuccess := atomic.LoadUint64(&successCount)
	reqError := atomic.LoadUint64(&errorCount)

	rps := float64(reqTotal) / actualDuration.Seconds()

	fmt.Printf("Test Completed!\n\n")
	fmt.Printf("Results:\n")
	fmt.Printf("----------------------------------------------------------\n")
	fmt.Printf("Total Requests:     %d\n", reqTotal)
	fmt.Printf("Successful:         %d\n", reqSuccess)
	fmt.Printf("Failed:             %d\n", reqError)
	fmt.Printf("Time Taken:         %.2f seconds\n", actualDuration.Seconds())
	fmt.Printf("Requests/Second:    %.2f RPS\n", rps)
	fmt.Printf("Success Rate:       %.2f%%\n", float64(reqSuccess)/float64(reqTotal)*100)
	fmt.Printf("----------------------------------------------------------\n")
	fmt.Printf("\nCheck the dashboard at http://localhost:8090 to see the internal metrics.\n")
}

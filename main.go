package main

import(
	"time"
)
func main() {
	InitDB()
	LoadVisitedFromDB()

	wg.Add(1)
	go func() {
		defer wg.Done()
		QueueLinks([]string{"https://en.wikipedia.org/wiki/Main_Page"})
		Crawl()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			time.Sleep(5 * time.Minute) 
			ComputePageRank(20, 0.85)
		}
	}()

	wg.Wait()
}

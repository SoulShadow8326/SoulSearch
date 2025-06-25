package main

func main(){
	InitDB()
	seed := "https://en.wikipedia.org/wiki/Main_Page"
	wg.Add(1)
	go func ()  {
		defer wg.Done()
		Crawl(seed)
	}()
	wg.Wait()
}
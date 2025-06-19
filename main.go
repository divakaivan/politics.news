package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

type RSS struct {
	Channel RSSFeed `xml:"channel"`
}

type RSSFeed struct {
	Title         string    `xml:"title"`
	Link          string    `xml:"link"`
	Description   string    `xml:"description"`
	LastBuildDate string    `xml:"lastBuildDate"`
	Items         []RSSItem `xml:"item"`
}

type RSSItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	Id          string `xml:"guid"`
	PublishDate string `xml:"pubDate"`
	Creator     string `xml:"dc:creator"`
}

func scrapeUrlFeed(url string) (RSSFeed, error) {
	httpClient := http.Client{
		Timeout: time.Second * 2,
	}
	fmt.Println("Fetching feed", url)
	resp, err := httpClient.Get(url)
	if err != nil {
		return RSSFeed{}, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return RSSFeed{}, err
	}
	rss := RSS{}
	err = xml.Unmarshal(data, &rss)
	if err != nil {
		return RSSFeed{}, err
	}
	return rss.Channel, nil
}

func main() {
	url := "https://rss.politico.com/playbook.xml"
	feed, err := scrapeUrlFeed(url)
	if err != nil {
		log.Fatalf("Failed to fetch feed: %v", err)
	}

	fmt.Printf("Feed: %s\n\n", feed.Title)
	for _, item := range feed.Items {
		fmt.Println("Title:", item.Title)
		fmt.Println("Link:", item.Link)
		fmt.Println("Description:", item.Description)
		fmt.Println("Published:", item.PublishDate)
		fmt.Println("Author:", item.Creator)
		fmt.Println("-----")
	}
}

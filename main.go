package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
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

// satisfy the bubbles list interface
type rssListItem struct {
	title string
	desc  string
	link  string
}

func (r rssListItem) Title() string       { return r.title }
func (r rssListItem) Description() string { return r.desc }
func (r rssListItem) FilterValue() string { return r.title }

// ---------------------------------

func toListItems(items []RSSItem) []list.Item {
	l := make([]list.Item, len(items))
	for i, item := range items {
		l[i] = rssListItem{
			title: item.Title,
			desc:  item.Description,
			link:  item.Link,
		}
	}
	return l
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

func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "linux":
		cmd = "xdg-open"
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler"}
	case "darwin":
		cmd = "open"
	default:
		return fmt.Errorf("unsupported platform")
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}

var docStyle = lipgloss.NewStyle() //.Margin(1, 2)

var modalStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("63")).Padding(1, 2).Width(60).Align(0)

type model struct {
	list       list.Model
	showDetail bool
	selected   rssListItem
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if msg.String() == "enter" {
			if item, ok := m.list.SelectedItem().(rssListItem); ok {
				m.showDetail = true
				m.selected = item
			}
			return m, nil
		}
		if msg.String() == "esc" {
			m.showDetail = false
			return m, nil
		}

		if msg.String() == "o" {
			if m.showDetail {
				go openBrowser(m.selected.link)
			}
			return m, nil
		}
	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) View() string {
	listView := docStyle.Render(m.list.View())
	if m.showDetail {
		out, err := glamour.Render(
			fmt.Sprintf("# %s\n\n%s\n\n[Source](%s)\n\n*Press 'o' to open in browser, press Esc to go back.*",
				m.selected.title,
				m.selected.desc,
				m.selected.link,
			), "dark")
		if err != nil {
			log.Printf("Failed to render markdown: %v", err)
			return listView
		}
		modal := modalStyle.Render(out)
		return listView + "\n\n" + modal
	}
	return listView
}

func main() {
	url := "https://rss.politico.com/playbook.xml"
	feed, err := scrapeUrlFeed(url)
	if err != nil {
		log.Fatalf("Failed to fetch feed: %v", err)
	}
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true

	fmt.Println("Fetched", len(feed.Items), "items from feed")

	m := model{
		list: list.New(toListItems(feed.Items), delegate, 0, 0),
	}
	m.list.Title = feed.Title
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
}

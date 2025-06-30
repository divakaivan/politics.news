package main

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	"github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"
)

const (
	host    = "localhost"
	port    = "23234"
	feedURL = "https://rss.politico.com/playbook.xml"
)

func teaHandler(s ssh.Session) (tea.Model, []tea.ProgramOption) {
	pty, _, _ := s.Pty()
	feed, err := scrapeUrlFeed(feedURL)
	if err != nil {
		log.Error("Failed to fetch feed", "error", err)
		return model{}, []tea.ProgramOption{tea.WithAltScreen()}
	}
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	m := model{list: list.New(toListItems(feed.Items), delegate, pty.Window.Width, pty.Window.Height)}
	m.list.Title = feed.Title
	return m, []tea.ProgramOption{tea.WithAltScreen()}
}

func main() {
	s, err := wish.NewServer(
		wish.WithAddress(net.JoinHostPort(host, port)),
		wish.WithHostKeyPath(".ssh/id_ed25519"),
		wish.WithMiddleware(
			bubbletea.Middleware(teaHandler),
			activeterm.Middleware(),
			logging.Middleware(),
		),
	)
	if err != nil {
		log.Error("Could not start server", "error", err)
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	log.Info("Starting SSH server", "host", host, "port", port)
	go func() {
		if err = s.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			log.Error("Could not start server", "error", err)
			done <- nil
		}
	}()

	<-done
	log.Info("Stopping SSH server")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
		log.Error("Could not stop server", "error", err)
	}
}

// model and list helpers

type rssListItem struct {
	title string
	desc  string
	link  string
}

func (r rssListItem) Title() string       { return r.title }
func (r rssListItem) Description() string { return r.desc }
func (r rssListItem) FilterValue() string { return r.title }

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
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "enter":
			if item, ok := m.list.SelectedItem().(rssListItem); ok {
				m.showDetail = true
				m.selected = item
			}
			return m, nil
		case "esc":
			m.showDetail = false
			return m, nil
		case "o":
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
			slog.Default().Error("Failed to render markdown", "error", err)
			return listView
		}
		modal := modalStyle.Render(out)
		return listView + "\n\n" + modal
	}
	return listView
}

func toListItems(items []RSSItem) []list.Item {
	l := make([]list.Item, len(items))
	for i, item := range items {
		l[i] = rssListItem{title: item.Title, desc: item.Description, link: item.Link}
	}
	return l
}

// RSS parsing

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
	httpClient := http.Client{Timeout: 2 * time.Second}
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
	if err := xml.Unmarshal(data, &rss); err != nil {
		return RSSFeed{}, err
	}
	return rss.Channel, nil
}

// styles

var (
	docStyle   = lipgloss.NewStyle()
	modalStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("63")).Padding(1, 2).Width(60).Align(lipgloss.Left)
)

// optional browser opening

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

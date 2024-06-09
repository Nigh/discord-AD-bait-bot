package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "time/tzdata"

	kuma "github.com/Nigh/kuma-push"
	"github.com/bwmarrin/discordgo"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	scribble "github.com/nanobox-io/golang-scribble"
)

var (
	gKumaPushURL   string
	gToken         string
	gBaitChannelID string
)

func kumaInit() {
	k := kuma.New(gKumaPushURL)
	k.Start()
}

var msgLog *log.Logger
var errLog *log.Logger

func logInit() {
	_, debug := os.LookupEnv("DEBUG")
	if debug {
		log.SetReportCaller(true)
		log.SetLevel(log.DebugLevel)
	}
	log.SetTimeFormat(time.TimeOnly)
	msgLog = log.WithPrefix("on msg")
	errLog = log.NewWithOptions(os.Stderr, log.Options{
		ReportTimestamp: true,
		ReportCaller:    true,
		TimeFormat:      "15:04:05.999999999",
	})
	styles := log.DefaultStyles()
	styles.Levels[log.ErrorLevel] = lipgloss.NewStyle().
		SetString("ERROR").
		Padding(0, 1, 0, 1).
		Background(lipgloss.Color("204")).
		Foreground(lipgloss.Color("0"))
	styles.Levels[log.FatalLevel] = lipgloss.NewStyle().
		SetString("FATAL").
		Padding(0, 1, 0, 1).
		Background(lipgloss.Color("1")).
		Foreground(lipgloss.Color("0"))

	errLog.SetStyles(styles)
}

type testenv struct {
	Token         string `json:"token"`
	KumaURL       string `json:"kumaurl"`
	BaitChannelID string `json:"baitchannelid"`
}

var testEnv testenv

var db *scribble.Driver

func envInit() {
	db, _ = scribble.New("../db", nil)
	db.Read("test", "env", &testEnv)
	if testEnv.Token != "" {
		gToken = testEnv.Token
		gBaitChannelID = testEnv.BaitChannelID
		gKumaPushURL = testEnv.KumaURL
	} else {
		gToken = os.Getenv("BOT_TOKEN")
		gBaitChannelID = os.Getenv("BAIT_CHANNEL")
		gKumaPushURL = os.Getenv("KUMA_PUSH_URL")
	}
	log.Debug("Read ENV", "gToken", gToken, "gBaitChannelID", gBaitChannelID, "gKumaPushURL", gKumaPushURL)
}

func init() {
	logInit()
	envInit()
	kumaInit()
	msgCacheInit()
}

func ready(s *discordgo.Session, event *discordgo.Ready) {
	log.Info("Bot is ready!")
}

type Msg struct {
	ChannelID string
	MessageID string
	UserID    string
}

type MsgCache struct {
	Msg
	TimeoutSec int
}

var msgCache []MsgCache

func msgCacheInit() {
	msgCache = make([]MsgCache, 0)
	go msgCacheTimer()
}

func msgCacheTimer() {
	interval := time.NewTicker(1 * time.Second)
	defer interval.Stop()
	for range interval.C {
		newMsgCache := make([]MsgCache, 0)
		for i := range msgCache {
			msgCache[i].TimeoutSec--
			if msgCache[i].TimeoutSec > 0 {
				newMsgCache = append(newMsgCache, msgCache[i])
			}
		}
		msgCache = newMsgCache
	}
}

func msgCacheAdd(m *discordgo.MessageCreate) {
	msgCache = append(msgCache, MsgCache{Msg: Msg{ChannelID: m.ChannelID, MessageID: m.ID, UserID: m.Author.ID}, TimeoutSec: 60})
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	msgLog.Info("New message", "uid", m.Author.ID, "name", m.Author.Username, "channel", m.ChannelID)
	msgCacheAdd(m)
	if m.ChannelID == gBaitChannelID {
		msgLog.Warn("ADS catched", "uid", m.Author.ID, "content", m.Content)
		err := s.GuildBanCreateWithReason(m.GuildID, m.Author.ID, "Ads bot", 0)
		if err != nil {
			errLog.Error("GuildBanCreateWithReason", "err", err)
		}
		newMsgCache := make([]MsgCache, 0)
		for i := range msgCache {
			if msgCache[i].UserID == m.Author.ID {
				log.Info("Delete message", "channel", msgCache[i].ChannelID, "message", msgCache[i].MessageID)
				err = s.ChannelMessageDelete(msgCache[i].ChannelID, msgCache[i].MessageID)
				if err != nil {
					newMsgCache = append(newMsgCache, msgCache[i])
					errLog.Error("ChannelMessageDelete", "channel", msgCache[i].ChannelID, "message", msgCache[i].MessageID, "err", err)
				}
			} else {
				newMsgCache = append(newMsgCache, msgCache[i])
			}
		}
		msgCache = newMsgCache
	}
}

func main() {
	dg, err := discordgo.New("Bot " + gToken)
	if err != nil {
		errLog.Fatal("error creating Discord session", "err", err)
	}

	dg.AddHandler(ready)
	dg.AddHandler(messageCreate)
	err = dg.Open()
	if err != nil {
		errLog.Fatal("error opening connection", "err", err)
	}
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
	dg.Close()
	log.Info("Bot OFFLINE")
}

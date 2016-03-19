package main

import (
	"github.com/bwmarrin/discordgo"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const myUserID = "160807650345353226"

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	fmt.Printf("%20s %20s %20s > %s\n", m.ChannelID, time.Now().Format(time.Stamp), m.Author.Username, m.Content)
	if m.Author.ID == myUserID {
		return
	}
	channel, err := s.Channel(m.ChannelID)
	if (err != nil) {
		return
	}
	if channel.IsPrivate {
		s.ChannelMessageSend(m.ChannelID, "wan sum fuk?")
		return
	}
	if strings.Contains(m.Content, "<@" + myUserID + "> lirik") {
		lirik := exec.Command("/home/ross/markov/1-markov.out", "1")
		lirikLogs, err := os.Open("/home/ross/markov/lirik")
		if err != nil {
			fmt.Println(err)
			return
		}
		lirik.Stdin = lirikLogs
		lirikout, err := lirik.Output()
		if err != nil {
			fmt.Println(err)
			return
		}
		s.ChannelMessageSend(m.ChannelID, string(lirikout))
	}
}

func main() {
	if err != nil {
		fmt.Println(err)
		return
	}
	client.AddHandler(messageCreate)
	client.Open()

	var input string
	fmt.Scanln(&input)
	return
}

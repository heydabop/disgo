package main

import (
	"github.com/bwmarrin/discordgo"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type command func([]string) (string, error)

func twitch(args []string) (string, error) {
	cmd := exec.Command("/home/ross/markov/1-markov.out", "1")
	logs, err := os.Open("/home/ross/markov/" + args[0] + "_nolink")
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	cmd.Stdin = logs
	out, err := cmd.Output()
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func soda(args []string) (string, error) {
	return twitch([]string{"sodapoppin"})
}

func lirik(args []string) (string, error) {
	return twitch([]string{"lirik"})
}

func forsen(args []string) (string, error) {
	return twitch([]string{"forsenlol"})
}

func makeMessageCreate() func(*discordgo.Session, *discordgo.MessageCreate) {
	const myUserID = "160807650345353226"
	regexes := []*regexp.Regexp{regexp.MustCompile(`^<@` + myUserID + `>\s+(.+)`), regexp.MustCompile(`^\/(.+)`)}
	funcMap := map[string]command{
		"twitch": command(twitch),
		"soda": command(soda),
		"lirik": command(lirik),
		"forsen": command(forsen),
	}

	return func (s *discordgo.Session, m *discordgo.MessageCreate) {
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
		for _, regex := range regexes {
			if match := regex.FindStringSubmatch(m.Content); match != nil {
				command := strings.Fields(match[1])
				if cmd, valid := funcMap[strings.ToLower(command[0])]; valid {
					reply, err := cmd(command[1:])
					if err != nil {
						return
					}
					s.ChannelMessageSend(m.ChannelID, reply)
					return
				}
			}
		}
	}
}

func main() {

	if err != nil {
		fmt.Println(err)
		return
	}
	client.AddHandler(makeMessageCreate())
	client.Open()

	var input string
	fmt.Scanln(&input)
	return
}

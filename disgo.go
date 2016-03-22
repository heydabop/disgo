package main

import (
	"errors"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/fzzy/radix/redis"
	"math/rand"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type command func([]string) (string, error)

var redisClient *redis.Client

func twitch(args []string) (string, error) {
	if len(args) < 1 {
		return "", errors.New("No channel name provided")
	}
	cmd := exec.Command("/home/ross/markov/1-markov.out", "1")
	logs, err := os.Open("/home/ross/markov/" + args[0] + "_nolink")
	if err != nil {
		return "", err
	}
	cmd.Stdin = logs
	out, err := cmd.Output()
	if err != nil {
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

func vote(args []string, inc int64) (string, error) {
	if len(args) < 1 {
		return "", errors.New("No userId provided")
	}
	userMention := args[0]
	userIdRegex := regexp.MustCompile(`<@(\d+?)>`)
	var userId string
	if match := userIdRegex.FindStringSubmatch(userMention); match != nil {
		userId = match[1]
	} else {
		return "", errors.New("No valid mention found")
	}
	redisKey := "disgo-userKarma-" + userId
	karma := redisClient.Cmd("GET", redisKey)
	if karma.Err != nil {
		return "", karma.Err
	}
	if karma.Type == redis.NilReply {
		redisClient.Cmd("set", redisKey, 0+inc)
	} else {
		karmaVal, err := karma.Int64()
		if err != nil {
			return "", err
		}
		karmaVal += inc
		redisClient.Cmd("set", redisKey, karmaVal)
	}
	return "", nil
}

func upvote(args []string) (string, error) {
	return vote(args, 1)
}

func downvote(args []string) (string, error) {
	return vote(args, -1)
}

func roll(args []string) (string, error) {
	var max int
	if len(args) < 1 {
		max = 6
	} else {
		var err error
		max, err = strconv.Atoi(args[0])
		if err != nil || max < 0 {
			return "", err
		}
	}
	return strconv.Itoa(rand.Intn(max) + 1), nil
}

func help(args []string) (string, error) {
	return "twitch [streamer], soda, lirik, forsen, roll [sides (optional)]", nil
}

func makeMessageCreate() func(*discordgo.Session, *discordgo.MessageCreate) {
	const myUserID = "160807650345353226"
	regexes := []*regexp.Regexp{regexp.MustCompile(`^<@` + myUserID + `>\s+(.+)`), regexp.MustCompile(`^\/(.+)`)}
	funcMap := map[string]command{
		"twitch":   command(twitch),
		"soda":     command(soda),
		"lirik":    command(lirik),
		"forsen":   command(forsen),
		"roll":     command(roll),
		"help":     command(help),
		"upvote":   command(upvote),
		"downvote": command(downvote),
	}

	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		fmt.Printf("%20s %20s %20s > %s\n", m.ChannelID, time.Now().Format(time.Stamp), m.Author.Username, m.Content)
		if m.Author.ID == myUserID {
			return
		}
		channel, err := s.Channel(m.ChannelID)
		if err != nil {
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
						fmt.Println("ERROR: " + err.Error())
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
	var err error
	redisClient, err = redis.Dial("tcp", "127.0.0.1:6379")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

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

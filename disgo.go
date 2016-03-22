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

type command func(string, string, []string) (string, error)

var redisClient *redis.Client
var voteTime map[string]time.Time = make(map[string]time.Time)

func twitch(chanId, authorId string, args []string) (string, error) {
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

func soda(chanId, authorId string, args []string) (string, error) {
	return twitch(chanId, authorId, []string{"sodapoppin"})
}

func lirik(chanId, authorId string, args []string) (string, error) {
	return twitch(chanId, authorId, []string{"lirik"})
}

func forsen(chanId, authorId string, args []string) (string, error) {
	return twitch(chanId, authorId, []string{"forsenlol"})
}

func vote(chanId, authorId string, args []string, inc int64) (string, error) {
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
	if authorId == userId {
		return "No.", nil
	}
	lastVoteTime, validTime := voteTime[authorId]
	if validTime && time.Since(lastVoteTime).Minutes() < 5 {
		return "Slow down champ.", nil
	}
	redisKey := fmt.Sprintf("disgo-userKarma-%s-%s", chanId, userId)
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
	voteTime[authorId] = time.Now()
	return "", nil
}

func upvote(chanId, authorId string, args []string) (string, error) {
	return vote(chanId, authorId, args, 1)
}

func downvote(chanId, authorId string, args []string) (string, error) {
	return vote(chanId, authorId, args, -1)
}

func roll(chanId, authorId string, args []string) (string, error) {
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

func help(chanId, authorId string, args []string) (string, error) {
	return "twitch [streamer], soda, lirik, forsen, roll [sides (optional)], upvote [@user] (or @user++), downvote [@user] (or @user--)", nil
}

func makeMessageCreate() func(*discordgo.Session, *discordgo.MessageCreate) {
	const myUserID = "160807650345353226"
	regexes := []*regexp.Regexp{regexp.MustCompile(`^<@` + myUserID + `>\s+(.+)`), regexp.MustCompile(`^\/(.+)`)}
	upvoteRegex := regexp.MustCompile(`(<@\d+?>)\s*\+\+`)
	downvoteRegex := regexp.MustCompile(`(<@\d+?>)\s*--`)
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
		var command []string
		if match := upvoteRegex.FindStringSubmatch(m.Content); match != nil {
			command = []string{"upvote", match[1]}
		}
		if len(command) == 0 {
			if match := downvoteRegex.FindStringSubmatch(m.Content); match != nil {
				command = []string{"downvote", match[1]}
			}
		}
		if len(command) == 0 {
			for _, regex := range regexes {
				if match := regex.FindStringSubmatch(m.Content); match != nil {
					command = strings.Fields(match[1])
					break
				}
			}
		}
		if len(command) == 0 {
			return
		}
		if cmd, valid := funcMap[strings.ToLower(command[0])]; valid {
			reply, err := cmd(m.ChannelID, m.Author.ID, command[1:])
			if err != nil {
				fmt.Println("ERROR: " + err.Error())
				return
			}
			if len(reply) > 0 {
				s.ChannelMessageSend(m.ChannelID, reply)
			}
			return
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

package main

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/fzzy/radix/redis"
	_ "github.com/mattn/go-sqlite3"
	"math/rand"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Command func(string, string, []string) (string, error)
type UserVote struct {
	userId string
	votes  int64
}
type UserVotes []UserVote

func (u UserVotes) Len() int {
	return len(u)
}
func (u UserVotes) Less(i, j int) bool {
	return u[i].votes-u[j].votes > 0
}
func (u UserVotes) Swap(i, j int) {
	u[i], u[j] = u[j], u[i]
}

const myUserID = "160807650345353226"

var sqlClient *sql.DB
var redisClient *redis.Client
var voteTime map[string]time.Time = make(map[string]time.Time)
var userIdRegex = regexp.MustCompile(`<@(\d+?)>`)

func twitch(chanId, authorId string, args []string) (string, error) {
	if len(args) < 1 {
		cmd := exec.Command("find", "-iname", "*_nolink")
		cmd.Dir = "/home/ross/markov/"
		out, err := cmd.Output()
		if err != nil {
			return "", err
		}
		files := strings.Fields(string(out))
		for i := range files {
			files[i] = strings.Replace(files[i], "./", "", 1)
			files[i] = strings.Replace(files[i], "_nolink", "", 1)
		}
		return strings.Join(files, ", "), nil
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
	var userId string
	if match := userIdRegex.FindStringSubmatch(userMention); match != nil {
		userId = match[1]
	} else {
		return "", errors.New("No valid mention found")
	}
	if authorId != myUserID {
		lastVoteTime, validTime := voteTime[authorId]
		if validTime && time.Since(lastVoteTime).Minutes() < 5 {
			return "Slow down champ.", nil
		}
	}
	if authorId == userId {
		if inc > 0 {
			_, err := vote(chanId, myUserID, []string{"<@" + authorId + ">"}, -1)
			if err != nil {
				return "", err
			}
			voteTime[authorId] = time.Now()
		}
		return "No.", nil
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

func votes(chanId, authorId string, args []string) (string, error) {
	if len(args) > 0 {
		var userId string
		fmt.Println(args[0])
		if match := userIdRegex.FindStringSubmatch(args[0]); match != nil {
			userId = match[1]
		} else {
			return "", errors.New("No valid mention found")
		}
		karma := redisClient.Cmd("get", fmt.Sprintf("disgo-userKarma-%s-%s", chanId, userId))
		if karma.Err != nil {
			return "", karma.Err
		}
		karmaStr, err := karma.Str()
		if err != nil {
			return "", err
		}
		return karmaStr, nil
	} else {
		keys := redisClient.Cmd("keys", fmt.Sprintf("disgo-userKarma-%s*", chanId))
		if keys.Err != nil {
			return "", keys.Err
		}
		votes := make(UserVotes, 0)
		keyStrings, err := keys.List()
		if err != nil {
			return "", err
		}
		for _, key := range keyStrings {
			karma := redisClient.Cmd("get", key)
			if karma.Err != nil {
				return "", karma.Err
			}
			karmaVal, err := karma.Int64()
			if err != nil {
				return "", err
			}
			var userId string
			userKeyRegex := regexp.MustCompile(`disgo-userKarma-` + chanId + `-(\d+)`)
			if match := userKeyRegex.FindStringSubmatch(key); match != nil {
				userId = match[1]
			} else {
				return "", errors.New("No userId found in redis key")
			}
			votes = append(votes, UserVote{userId, karmaVal})
		}
		sort.Sort(&votes)
		finalString := ""
		for i, vote := range votes {
			if i >= 5 {
				break
			}
			finalString += fmt.Sprintf("<@%s>: %d, ", vote.userId, vote.votes)
		}
		if len(finalString) > 0 {
			return finalString[:len(finalString)-2], nil
		} else {
			return "", nil
		}
	}
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
	return "twitch [streamer (optional)], soda, lirik, forsen, roll [sides (optional)], upvote [@user] (or @user++), downvote [@user] (or @user--), karma/votes [@user (optional)", nil
}

func makeMessageCreate() func(*discordgo.Session, *discordgo.MessageCreate) {
	regexes := []*regexp.Regexp{regexp.MustCompile(`^<@` + myUserID + `>\s+(.+)`), regexp.MustCompile(`^\/(.+)`)}
	upvoteRegex := regexp.MustCompile(`(<@\d+?>)\s*\+\+`)
	downvoteRegex := regexp.MustCompile(`(<@\d+?>)\s*--`)
	funcMap := map[string]Command{
		"twitch":   Command(twitch),
		"soda":     Command(soda),
		"lirik":    Command(lirik),
		"forsen":   Command(forsen),
		"roll":     Command(roll),
		"help":     Command(help),
		"upvote":   Command(upvote),
		"downvote": Command(downvote),
		"votes":    Command(votes),
		"karma":    Command(votes),
	}

	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		now := time.Now()
		fmt.Printf("%20s %20s %20s > %s\n", m.ChannelID, now.Format(time.Stamp), m.Author.Username, m.Content)
		_, err := sqlClient.Exec("INSERT INTO messages (ChanId, AuthorId, Timestamp, Message) values (?, ?, ?, ?)",
			m.ChannelID, m.Author.ID, now.Format(time.RFC3339Nano), m.Content)
		if err != nil {
			fmt.Println("ERROR inserting into messages db")
			fmt.Println(err.Error())
		}
		if m.Author.ID == myUserID {
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
				fmt.Println("ERROR in " + command[0])
				fmt.Printf("ARGS: %v\n", command[1:])
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
	sqlClient, err = sql.Open("sqlite3", "disgo.db")
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

package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/bwmarrin/discordgo"
	_ "github.com/mattn/go-sqlite3"
	"io/ioutil"
	"math"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Command func(*discordgo.Session, string, string, []string) (string, error)
type KarmaDto struct {
	UserId string
	Karma  int64
}
type TwitchChannel struct {
	DisplayName string `json:"display_name"`
	Name        string `json:"name"`
	Status      string `json:"status"`
}
type TwitchStream struct {
	Id          int           `json:"_id"`
	AverageFps  float64       `json:"average_fps"`
	Game        string        `json:"game"`
	Viewers     int           `json:"viewers"`
	Channel     TwitchChannel `json:"channel"`
	VideoHeight int           `json:"video_height"`
}
type TwitchStreamReply struct {
	Stream *TwitchStream `json:"stream"`
}
type UserMessageCount struct {
	AuthorId    string
	NumMessages int64
}
type UserMessageLength struct {
	AuthorId  string
	AvgLength float64
}

var sqlClient *sql.DB
var voteTime map[string]time.Time = make(map[string]time.Time)
var userIdRegex = regexp.MustCompile(`<@(\d+?)>`)
var typingTimer map[string]*time.Timer = make(map[string]*time.Timer)
var currentVoiceChannel = ""
var currentVoiceGuild = ""
var ownUserId = ""

func spam(session *discordgo.Session, chanId, authorId string, args []string) (string, error) {
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

func soda(session *discordgo.Session, chanId, authorId string, args []string) (string, error) {
	return spam(session, chanId, authorId, []string{"sodapoppin"})
}

func lirik(session *discordgo.Session, chanId, authorId string, args []string) (string, error) {
	return spam(session, chanId, authorId, []string{"lirik"})
}

func forsen(session *discordgo.Session, chanId, authorId string, args []string) (string, error) {
	return spam(session, chanId, authorId, []string{"forsenlol"})
}

func vote(session *discordgo.Session, chanId, authorId string, args []string, inc int64) (string, error) {
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
	if authorId != ownUserId {
		lastVoteTime, validTime := voteTime[authorId]
		if validTime && time.Since(lastVoteTime).Minutes() < 5 {
			return "Slow down champ.", nil
		}
	}
	if authorId == userId {
		if inc > 0 {
			_, err := vote(session, chanId, ownUserId, []string{"<@" + authorId + ">"}, -1)
			if err != nil {
				return "", err
			}
			voteTime[authorId] = time.Now()
		}
		return "No.", nil
	}

	var karma int64
	err := sqlClient.QueryRow("select Karma from karma where ChanId = ? and UserId = ?", chanId, userId).Scan(&karma)
	if err != nil {
		if err == sql.ErrNoRows {
			karma = 0
			_, insertErr := sqlClient.Exec("insert into Karma(ChanId, UserId, Karma) values (?, ?, ?)", chanId, userId, karma)
			if insertErr != nil {
				return "", err
			}
		} else {
			return "", err
		}
	}

	karma += inc
	_, err = sqlClient.Exec("update karma set Karma = ? where ChanId = ? and UserId = ?", karma, chanId, userId)
	if err != nil {
		return "", err
	}

	voteTime[authorId] = time.Now()
	return "", nil
}

func upvote(session *discordgo.Session, chanId, authorId string, args []string) (string, error) {
	return vote(session, chanId, authorId, args, 1)
}

func downvote(session *discordgo.Session, chanId, authorId string, args []string) (string, error) {
	return vote(session, chanId, authorId, args, -1)
}

func votes(session *discordgo.Session, chanId, authorId string, args []string) (string, error) {
	if len(args) > 0 {
		var userId string
		fmt.Println(args[0])
		if match := userIdRegex.FindStringSubmatch(args[0]); match != nil {
			userId = match[1]
		} else {
			return "", errors.New("No valid mention found")
		}
		var karma int64
		err := sqlClient.QueryRow("select Karma from karma where ChanId = ? and UserId = ?", chanId, userId).Scan(&karma)
		if err != nil {
			return "", err
		}
		return strconv.FormatInt(karma, 10), nil
	} else {
		rows, err := sqlClient.Query("select UserId, Karma from karma where ChanId = ? order by Karma desc limit 5", chanId)
		if err != nil {
			return "", err
		}
		defer rows.Close()
		votes := make([]KarmaDto, 0)
		for rows.Next() {
			var userId string
			var karma int64
			err := rows.Scan(&userId, &karma)
			if err != nil {
				return "", err
			}
			votes = append(votes, KarmaDto{userId, karma})
		}
		finalString := ""
		for _, vote := range votes {
			user, err := session.User(vote.UserId)
			if err != nil {
				return "", err
			}
			finalString += fmt.Sprintf("%s: %d, ", user.Username, vote.Karma)
		}
		if len(finalString) >= 2 {
			return finalString[:len(finalString)-2], nil
		} else {
			return "", nil
		}
	}
}

func roll(session *discordgo.Session, chanId, authorId string, args []string) (string, error) {
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

func uptime(session *discordgo.Session, chanId, authorId string, args []string) (string, error) {
	output, err := exec.Command("uptime").Output()
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(string(output)), nil
}

func twitch(session *discordgo.Session, chanId, authorId string, args []string) (string, error) {
	if len(args) < 1 {
		return "", errors.New("No stream provided")
	}
	streamName := args[0]
	res, err := http.Get(fmt.Sprintf("https://api.twitch.tv/kraken/streams/%s", streamName))
	if err != nil {
		return "", err
	}
	body, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		return "", err
	}
	var reply TwitchStreamReply
	err = json.Unmarshal(body, &reply)
	if err != nil {
		return "", err
	}
	if reply.Stream == nil {
		return "[Offline]", nil
	}
	return fmt.Sprintf(`%s playing %s
%s
%d viewers; %dp @ %.f FPS`, reply.Stream.Channel.Name, reply.Stream.Game, reply.Stream.Channel.Status, reply.Stream.Viewers, reply.Stream.VideoHeight, math.Floor(reply.Stream.AverageFps+0.5)), nil
}

func top(session *discordgo.Session, chanId, authorId string, args []string) (string, error) {
	var limit int
	if len(args) < 1 {
		limit = 5
	} else {
		var err error
		limit, err = strconv.Atoi(args[0])
		if err != nil || limit < 0 {
			return "", err
		}
	}
	rows, err := sqlClient.Query("select AuthorId, count(AuthorId) as NumMessages from messages where ChanId = ? group by AuthorId order by count(AuthorId) desc limit ?", chanId, limit)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	counts := make([]UserMessageCount, 0)
	for rows.Next() {
		var authorId string
		var numMessages int64
		err := rows.Scan(&authorId, &numMessages)
		if err != nil {
			return "", err
		}
		counts = append(counts, UserMessageCount{authorId, numMessages})
	}
	finalString := ""
	for _, count := range counts {
		user, err := session.User(count.AuthorId)
		if err != nil {
			return "", err
		}
		finalString += fmt.Sprintf("%s: %d, ", user.Username, count.NumMessages)
	}
	if len(finalString) >= 2 {
		return finalString[:len(finalString)-2], nil
	} else {
		return "", nil
	}
}

func topLength(session *discordgo.Session, chanId, authorId string, args []string) (string, error) {
	var limit int
	if len(args) < 1 {
		limit = 5
	} else {
		var err error
		limit, err = strconv.Atoi(args[0])
		if err != nil || limit < 0 {
			return "", err
		}
	}
	//rows, err := sqlClient.Query(`select AuthorId, avg(length(message)) as avgLength from messages where ChanId = ? and trim(message) != '' and not message REGEXP '^https?:\/\/.*?\/[^[:space:]]*?$' group by AuthorId order by avgLength desc limit ?`, chanId, limit)
	rows, err := sqlClient.Query(`select AuthorId, avg(length(message)) as avgLength from messages where ChanId = ? and trim(message) != '' group by AuthorId order by avgLength desc limit ?`, chanId, limit)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	avgLengths := make([]UserMessageLength, 0)
	for rows.Next() {
		var authorId string
		var avgLength float64
		err := rows.Scan(&authorId, &avgLength)
		if err != nil {
			return "", err
		}
		avgLengths = append(avgLengths, UserMessageLength{authorId, avgLength})
	}
	finalString := ""
	for _, length := range avgLengths {
		user, err := session.User(length.AuthorId)
		if err != nil {
			return "", err
		}
		finalString += fmt.Sprintf("%s: %.2f, ", user.Username, length.AvgLength)
	}
	if len(finalString) >= 2 {
		return finalString[:len(finalString)-2], nil
	} else {
		return "", nil
	}
}

func help(session *discordgo.Session, chanId, authorId string, args []string) (string, error) {
	return "spam [streamer (optional)], soda, lirik, forsen, roll [sides (optional)], upvote [@user] (or @user++), downvote [@user] (or @user--), karma/votes [@user (optional), uptime, twitch [channel], top [number (optional)], topLength [number (optional)]", nil
}

func makeMessageCreate() func(*discordgo.Session, *discordgo.MessageCreate) {
	regexes := []*regexp.Regexp{regexp.MustCompile(`^<@` + ownUserId + `>\s+(.+)`), regexp.MustCompile(`^\/(.+)`)}
	upvoteRegex := regexp.MustCompile(`(<@\d+?>)\s*\+\+`)
	downvoteRegex := regexp.MustCompile(`(<@\d+?>)\s*--`)
	twitchRegex := regexp.MustCompile(`https?:\/\/(www.)?twitch.tv\/([[:alnum:]_]+)`)
	funcMap := map[string]Command{
		"spam":      Command(spam),
		"soda":      Command(soda),
		"lirik":     Command(lirik),
		"forsen":    Command(forsen),
		"roll":      Command(roll),
		"help":      Command(help),
		"upvote":    Command(upvote),
		"downvote":  Command(downvote),
		"votes":     Command(votes),
		"karma":     Command(votes),
		"uptime":    Command(uptime),
		"twitch":    Command(twitch),
		"top":       Command(top),
		"toplength": Command(topLength),
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

		if m.Author.ID == ownUserId {
			return
		}

		if typingTimer, valid := typingTimer[m.Author.ID]; valid {
			typingTimer.Stop()
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
			if match := twitchRegex.FindStringSubmatch(m.Content); match != nil {
				command = []string{"twitch", match[2]}
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
			if command[0] != "upvote" && command[0] != "downvote" {
				s.ChannelTyping(m.ChannelID)
			}
			reply, err := cmd(s, m.ChannelID, m.Author.ID, command[1:])
			if err != nil {
				s.ChannelMessageSend(m.ChannelID, ":warning:")
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

func gameUpdater(s *discordgo.Session, ticker <-chan time.Time) {
	currentGame := ""
	games := []string{"Skynet Simulator 2020", "Kill All Humans", "WW III: The Game", "9GAG Meme Generator", "Subreddit Simulator",
		"Runescape", "War Games", "Half Life 3", "Secret of the Magic Crystals", "Dransik", "<Procedurally Generated Name>",
		"Call of Duty 3", "Dino D-Day", "Overwatch", "Euro Truck Simulator 2", "Farmville", "Dwarf Fortress",
		"Pajama Sam: No Need to Hide When It's Dark Outside", "League of Legends", "The Ship", "Sleepy Doge", "Surgeon Simulator",
		"Farming Simulator 2018: The Farming"}
	for {
		select {
		case <-ticker:
			if currentGame != "" {
				changeGame := rand.Intn(2)
				if changeGame != 0 {
					continue
				}
				currentGame = ""
			} else {
				index := rand.Intn(len(games) * 5)
				if index >= len(games) {
					currentGame = ""
				} else {
					currentGame = games[index]
				}
			}
			s.UpdateStatus(0, currentGame)
		}
	}
}

func main() {
	var err error
	sqlClient, err = sql.Open("sqlite3", "sqlite.db")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	client, err := discordgo.New(LOGIN_EMAIL, LOGIN_PASSWORD)
	if err != nil {
		fmt.Println(err)
		return
	}
	self, err := client.User("@me")
	if err != nil {
		fmt.Println(err)
		return
	}
	ownUserId = self.ID
	client.AddHandler(makeMessageCreate())
	/*client.AddHandler(func(s *discordgo.Session, v *discordgo.VoiceStateUpdate) {
		fmt.Printf("VOICE: %s %s %s\n", v.UserID, v.SessionID, v.ChannelID)
		if len(v.ChannelID) == 0 && v.UserID == ownUserId {
			currentVoiceChannel = ""
			currentVoiceGuild = ""
		}
		if len(v.ChannelID) == 0 {
			return
		}
		if v.UserID == ownUserId {
			if len(currentVoiceChannel) > 0 && currentVoiceChannel != v.ChannelID {
				s.ChannelVoiceJoin(currentVoiceGuild, currentVoiceChannel, true, false)
			}
			return
		}
		if rand.Intn(20) == 0 {
			channel, err := s.Channel(v.ChannelID)
			if err != nil {
				fmt.Println(err.Error())
				return
			}
			err = s.ChannelVoiceJoin(channel.GuildID, v.ChannelID, true, false)
			if err != nil {
				fmt.Println(err.Error())
				return
			}
			currentVoiceChannel = v.ChannelID
			currentVoiceGuild = channel.GuildID
			time.AfterFunc(time.Duration(rand.Int63n(1200)+600)*time.Second, func() {
				err := s.ChannelVoiceLeave()
				if err != nil {
					fmt.Println(err.Error())
				}
			})
		}
	})*/
	client.AddHandler(func(s *discordgo.Session, t *discordgo.TypingStart) {
		if t.UserID == ownUserId {
			return
		}
		if rand.Intn(20) == 0 {
			typingTimer[t.UserID] = time.AfterFunc(20*time.Second, func() {
				responses := []string{"Something to say?", "Yes?", "Don't leave us hanging...", "I'm listening."}
				responseId := rand.Intn(len(responses))
				s.ChannelMessageSend(t.ChannelID, fmt.Sprintf("<@%s> %s", t.UserID, responses[responseId]))
			})
		}
	})
	client.Open()

	gameTicker := time.NewTicker(817 * time.Second)
	go gameUpdater(client, gameTicker.C)

	var input string
	fmt.Scanln(&input)
	return
}

package main

import (
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/bwmarrin/dgvoice"
	"github.com/bwmarrin/discordgo"
	"github.com/gyuho/goling/similar"
	_ "github.com/mattn/go-sqlite3"
	"io/ioutil"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Command func(*discordgo.Session, string, string, string, []string) (string, error)
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

type WolframPlaintextPod struct {
	Title     string `xml:"title,attr"`
	Error     bool   `xml:"error,attr"`
	Primary   *bool  `xml:"primary,attr"`
	Plaintext string `xml:"subpod>plaintext"`
}

type WolframQueryResult struct {
	Success bool                  `xml:"success,attr"`
	Error   bool                  `xml:"error,attr"`
	NumPods int                   `xml:"numpods,attr"`
	Pods    []WolframPlaintextPod `xml:"pod"`
}

type UserMessageLengths []UserMessageLength

func (u UserMessageLengths) Len() int {
	return len(u)
}
func (u UserMessageLengths) Less(i, j int) bool {
	return u[i].AvgLength-u[j].AvgLength > 0
}
func (u UserMessageLengths) Swap(i, j int) {
	u[i], u[j] = u[j], u[i]
}

var sqlClient *sql.DB
var voteTime = make(map[string]time.Time)
var userIdRegex = regexp.MustCompile(`<@(\d+?)>`)
var typingTimer = make(map[string]*time.Timer)
var currentVoiceSession *discordgo.VoiceConnection
var currentVoiceTimer *time.Timer
var ownUserId = ""
var lastMessage discordgo.Message
var lastAuthorId = ""
var voiceMutex sync.Mutex
var Rand = rand.New(rand.NewSource(time.Now().UnixNano()))
var lastQuoteIds = make(map[string]int64)
var userIdUpQuotes = make(map[string][]string)

func getMostSimilarUserId(session *discordgo.Session, chanId, username string) (string, error) {
	channel, err := session.State.Channel(chanId)
	if err != nil {
		return "", err
	}
	guild, err := session.State.Guild(channel.GuildID)
	if err != nil {
		return "", err
	}
	var similarUsers []discordgo.User
	lowerUsername := strings.ToLower(username)
	if guild.Members != nil {
		for _, member := range guild.Members {
			if user := member.User; user != nil {
				if strings.Contains(strings.ToLower(user.Username), lowerUsername) {
					similarUsers = append(similarUsers, *user)
				}
			}
		}
	}
	if len(similarUsers) == 1 {
		return similarUsers[0].ID, nil
	}
	maxSim := 0.0
	maxUserId := ""
	usernameBytes := []byte(lowerUsername)
	for _, user := range similarUsers {
		sim := similar.Cosine([]byte(strings.ToLower(user.Username)), usernameBytes)
		if sim > maxSim {
			maxSim = sim
			maxUserId = user.ID
		}
	}
	if maxUserId != "" {
		return maxUserId, nil
	}
	maxSim = 0.0
	maxUserId = ""
	if guild.Members != nil {
		for _, member := range guild.Members {
			if user := member.User; user != nil {
				sim := similar.Cosine([]byte(strings.ToLower(user.Username)), usernameBytes)
				if sim > maxSim {
					maxSim = sim
					maxUserId = user.ID
				}
			}
		}
	}
	if maxUserId == "" {
		return "", errors.New("No similar user found")
	}
	return maxUserId, nil
}

func spam(session *discordgo.Session, chanId, authorId, messageId string, args []string) (string, error) {
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

func soda(session *discordgo.Session, chanId, authorId, messageId string, args []string) (string, error) {
	return spam(session, chanId, authorId, messageId, []string{"sodapoppin"})
}

func lirik(session *discordgo.Session, chanId, authorId, messageId string, args []string) (string, error) {
	return spam(session, chanId, authorId, messageId, []string{"lirik"})
}

func forsen(session *discordgo.Session, chanId, authorId, messageId string, args []string) (string, error) {
	return spam(session, chanId, authorId, messageId, []string{"forsenlol"})
}

func cwc(session *discordgo.Session, chanId, authorId, messageId string, args []string) (string, error) {
	return spam(session, chanId, authorId, messageId, []string{"cwc2016"})
}

func vote(session *discordgo.Session, chanId, authorId, messageId string, args []string, inc int64) (string, error) {
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
		if validTime && time.Since(lastVoteTime).Minutes() < 5+5*Rand.Float64() {
			return "Slow down champ.", nil
		}
	}
	if authorId == userId {
		if inc > 0 {
			_, err := vote(session, chanId, ownUserId, messageId, []string{"<@" + authorId + ">"}, -1)
			if err != nil {
				return "", err
			}
			voteTime[authorId] = time.Now()
		}
		return "No.", nil
	}
	channel, err := session.State.Channel(chanId)
	if err != nil {
		return "", err
	}

	var lastVoterIdAgainstUser, lastVoteTimestamp string
	var lastVoteTime time.Time
	err = sqlClient.QueryRow("select VoterId, Timestamp from Vote where GuildId = ? and VoteeId = ? order by Timestamp desc limit 1", channel.GuildID, authorId).Scan(&lastVoterIdAgainstUser, &lastVoteTimestamp)
	if err != nil {
		if err == sql.ErrNoRows {
			lastVoterIdAgainstUser = ""
		} else {
			return "", err
		}
	} else {
		lastVoteTime, err = time.Parse(time.RFC3339Nano, lastVoteTimestamp)
		if err != nil {
			return "", err
		}
	}
	if lastVoterIdAgainstUser == userId && time.Since(lastVoteTime).Hours() < 12 {
		return "Really?...", nil
	}
	var lastVoteeIdFromAuthor string
	err = sqlClient.QueryRow("select VoteeId, Timestamp from Vote where GuildId = ? and VoterId = ? order by Timestamp desc limit 1", channel.GuildID, authorId).Scan(&lastVoteeIdFromAuthor, &lastVoteTimestamp)
	if err != nil {
		if err == sql.ErrNoRows {
			lastVoteeIdFromAuthor = ""
		} else {
			return "", err
		}
	} else {
		lastVoteTime, err = time.Parse(time.RFC3339Nano, lastVoteTimestamp)
		if err != nil {
			return "", err
		}
	}
	if lastVoteeIdFromAuthor == userId && time.Since(lastVoteTime).Hours() < 12 {
		return "Really?...", nil
	}

	var karma int64
	err = sqlClient.QueryRow("select Karma from UserKarma where GuildId = ? and UserId = ?", channel.GuildID, userId).Scan(&karma)
	if err != nil {
		if err == sql.ErrNoRows {
			karma = 0
			_, insertErr := sqlClient.Exec("insert into UserKarma(GuildId, UserId, Karma) values (?, ?, ?)", channel.GuildID, userId, karma)
			if insertErr != nil {
				return "", err
			}
		} else {
			return "", err
		}
	}

	karma += inc
	_, err = sqlClient.Exec("update UserKarma set Karma = ? where GuildId = ? and UserId = ?", karma, channel.GuildID, userId)
	if err != nil {
		return "", err
	}
	voteTime[authorId] = time.Now()

	messageIdUnit, err := strconv.ParseUint(messageId, 10, 64)
	if err != nil {
		return "", err
	}
	isUpvote := false
	if inc > 0 {
		isUpvote = true
	}
	_, err = sqlClient.Exec("insert into Vote(GuildId, MessageId, VoterID, VoteeID, Timestamp, IsUpvote) values (?, ?, ?, ?, ?, ?)",
		channel.GuildID, messageIdUnit, authorId, userId, time.Now().Format(time.RFC3339Nano), isUpvote)
	if err != nil {
		return "", err
	}

	return "", nil
}

func upvote(session *discordgo.Session, chanId, authorId, messageId string, args []string) (string, error) {
	return vote(session, chanId, authorId, messageId, args, 1)
}

func downvote(session *discordgo.Session, chanId, authorId, messageId string, args []string) (string, error) {
	return vote(session, chanId, authorId, messageId, args, -1)
}

func votes(session *discordgo.Session, chanId, authorId, messageId string, args []string) (string, error) {
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
	channel, err := session.State.Channel(chanId)
	if err != nil {
		return "", err
	}
	rows, err := sqlClient.Query("select UserId, Karma from UserKarma where GuildId = ? order by Karma desc limit ?", channel.GuildID, limit)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var votes []KarmaDto
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
	}
	return "", nil
}

func roll(session *discordgo.Session, chanId, authorId, messageId string, args []string) (string, error) {
	var max int64
	if len(args) < 1 {
		max = 6
	} else {
		var err error
		max, err = strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return "", err
		}
		if max <= 0 {
			return "", errors.New("Max roll must be more than 0")
		}
	}
	return fmt.Sprintf("%d", Rand.Int63n(max)+1), nil
}

func uptime(session *discordgo.Session, chanId, authorId, messageId string, args []string) (string, error) {
	output, err := exec.Command("uptime").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func twitch(session *discordgo.Session, chanId, authorId, messageId string, args []string) (string, error) {
	if len(args) < 1 {
		return "", errors.New("No stream provided")
	}
	streamName := args[0]
	res, err := http.Get(fmt.Sprintf("https://api.twitch.tv/kraken/streams/%s", url.QueryEscape(streamName)))
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

func top(session *discordgo.Session, chanId, authorId, messageId string, args []string) (string, error) {
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
	rows, err := sqlClient.Query(`select AuthorId, count(AuthorId) as NumMessages from Message where ChanId = ? and Content not like '/%' group by AuthorId order by count(AuthorId) desc limit ?`, chanId, limit)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var counts []UserMessageCount
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
	}
	return "", nil
}

func topLength(session *discordgo.Session, chanId, authorId, messageId string, args []string) (string, error) {
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
	rows, err := sqlClient.Query(`select AuthorId, Content from Message where ChanId = ? and Content not like '/%' and trim(Content) != ''`, chanId)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	messagesPerUser := make(map[string]uint)
	wordsPerUser := make(map[string]uint)
	urlRegex := regexp.MustCompile(`^https?:\/\/.*?\/[^[:space:]]*?$`)
	for i := 0; rows.Next(); i++ {
		var authorId string
		var message string
		err := rows.Scan(&authorId, &message)
		if err != nil {
			return "", err
		}
		if urlRegex.MatchString(message) {
			continue
		}
		messagesPerUser[authorId]++
		wordsPerUser[authorId] += uint(len(strings.Fields(message)))
	}
	avgLengths := make(UserMessageLengths, 0)
	for userId, numMessages := range messagesPerUser {
		avgLengths = append(avgLengths, UserMessageLength{userId, float64(wordsPerUser[userId]) / float64(numMessages)})
	}
	sort.Sort(&avgLengths)
	finalString := ""
	for i, length := range avgLengths {
		if i >= limit {
			break
		}
		user, err := session.User(length.AuthorId)
		if err != nil {
			return "", err
		}
		finalString += fmt.Sprintf("%s: %.2f, ", user.Username, length.AvgLength)
	}
	if len(finalString) >= 2 {
		return finalString[:len(finalString)-2], nil
	}
	return "", nil
}

func rename(session *discordgo.Session, chanId, authorId, messageId string, args []string) (string, error) {
	if len(args) < 1 {
		return "", errors.New("No new username provided")
	}
	newUsername := strings.Join(args[0:], " ")
	var timestamp string
	var lockedMinutes int
	var lastChangeTime time.Time
	now := time.Now()
	err := sqlClient.QueryRow("select Timestamp, LockedMinutes from OwnUsername order by Timestamp desc limit 1").Scan(&timestamp, &lockedMinutes)
	if err != nil {
		if err == sql.ErrNoRows {
			lockedMinutes = 0
		} else {
			return "", err
		}
	} else {
		lastChangeTime, err = time.Parse(time.RFC3339Nano, timestamp)
		if err != nil {
			lastChangeTime = time.Now()
		}
	}

	if lockedMinutes == 0 || now.After(lastChangeTime.Add(time.Duration(lockedMinutes)*time.Minute)) {
		self, err := session.User("@me")
		if err != nil {
			return "", err
		}
		newSelf, err := session.UserUpdate(LOGIN_EMAIL, LOGIN_PASSWORD, newUsername, self.Avatar, "")
		if err != nil {
			return "", err
		}

		channel, err := session.State.Channel(chanId)
		if err != nil {
			return "", err
		}
		var authorKarma int
		err = sqlClient.QueryRow("select Karma from UserKarma where GuildId = ? and UserId = ?", channel.GuildID, authorId).Scan(&authorKarma)
		if err != nil {
			authorKarma = 0
		}
		newLockedMinutes := Rand.Intn(30) + 45 + 10*authorKarma
		if newLockedMinutes < 30 {
			newLockedMinutes = 30
		}

		_, err = sqlClient.Exec("INSERT INTO ownUsername (AuthorId, Timestamp, Username, LockedMinutes) values (?, ?, ?, ?)",
			authorId, now.Format(time.RFC3339Nano), newSelf.Username, newLockedMinutes)
		if err != nil {
			return "", err
		}
		author, err := session.User(authorId)
		if err != nil {
			return "", err
		}
		if authorKarma > 0 {
			return fmt.Sprintf("%s's name change will last for an extra %d minutes thanks to their karma!", author.Username, 10*authorKarma), nil
		} else if authorKarma < 0 {
			return fmt.Sprintf("%s's name change will last up to %d minutes less due to their karma...", author.Username, -10*authorKarma), nil
		}
	} else {
		return "I'm not ready to change who I am.", nil
	}
	return "", nil
}

func lastseen(session *discordgo.Session, chanId, authorId, messageId string, args []string) (string, error) {
	if len(args) < 1 {
		return "", errors.New("No username provided")
	}
	userId, err := getMostSimilarUserId(session, chanId, strings.Join(args, " "))
	if err != nil {
		return "", err
	}
	user, err := session.User(userId)
	if err != nil {
		return "", err
	}
	channel, err := session.State.Channel(chanId)
	if err != nil {
		return "", err
	}
	guild, err := session.State.Guild(channel.GuildID)
	if err != nil {
		return "", err
	}
	online := false
	for _, presence := range guild.Presences {
		if presence.User != nil && presence.User.ID == user.ID {
			online = presence.Status == "online"
			break
		}
	}
	if online {
		return fmt.Sprintf("%s is currently online", user.Username), nil
	}
	lastOnlineStr := ""
	err = sqlClient.QueryRow("select Timestamp from UserPresence where GuildId = ? and UserId = ? and (Presence = 'offline' or Presence = 'idle') order by Timestamp desc limit 1", guild.ID, userId).Scan(&lastOnlineStr)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Sprintf("%s was last seen at least %.f days ago", user.Username, time.Since(time.Date(2016, 4, 7, 1, 7, 0, 0, time.Local)).Hours()/24), nil
		}
		return "", err
	}
	lastOnline, err := time.Parse(time.RFC3339Nano, lastOnlineStr)
	if err != nil {
		return "", err
	}
	timeSince := time.Since(lastOnline)
	lastSeenStr := ""
	if timeSince <= 1*time.Second {
		lastSeenStr = "less than a second ago"
	} else if timeSince < 120*time.Second {
		lastSeenStr = fmt.Sprintf("%.f seconds ago", timeSince.Seconds())
	} else if timeSince < 120*time.Minute {
		lastSeenStr = fmt.Sprintf("%.f minutes ago", timeSince.Minutes())
	} else if timeSince < 48*time.Hour {
		lastSeenStr = fmt.Sprintf("%.f hours ago", timeSince.Hours())
	} else {
		lastSeenStr = fmt.Sprintf("%.f days ago", timeSince.Hours()/24)
	}
	return fmt.Sprintf("%s was last seen %s", user.Username, lastSeenStr), nil
}

func deleteLastMessage(session *discordgo.Session, chanId, authorId, messageId string, args []string) (string, error) {
	if lastAuthorId == authorId {
		err := session.ChannelMessageDelete(lastMessage.ChannelID, lastMessage.ID)
		if err != nil {
			return "", err
		}
	}
	return "", nil
}

func kickme(session *discordgo.Session, chanId, authorId, messageId string, args []string) (string, error) {
	channel, err := session.State.Channel(chanId)
	if err != nil {
		return "", err
	}
	err = session.GuildMemberDelete(channel.GuildID, authorId)
	if err != nil {
		return "", err
	}
	return "See ya nerd.", nil
}

func spamuser(session *discordgo.Session, chanId, authorId, messageId string, args []string) (string, error) {
	if len(args) < 1 {
		return "", errors.New("No userId provided")
	}
	userId, err := getMostSimilarUserId(session, chanId, strings.Join(args, " "))
	if err != nil {
		return "", err
	}
	user, err := session.User(userId)
	if err != nil {
		return "", err
	}
	err = exec.Command("bash", "./gen_custom_log.sh", chanId, userId).Run()
	if err != nil {
		return "", err
	}
	cmd := exec.Command("/home/ross/markov/1-markov.out", "1")
	logs, err := os.Open("/home/ross/markov/" + userId + "_custom")
	if err != nil {
		return "", err
	}
	cmd.Stdin = logs
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	outStr := strings.TrimSpace(string(out))
	if match := regexp.MustCompile(`^(.*) ([[:punct:]])$`).FindStringSubmatch(outStr); match != nil {
		outStr = match[1] + match[2]
	}
	var numRows int64
	err = sqlClient.QueryRow(`select Count(Id) from Message where Content like ? and AuthorId = ?;`, "%"+outStr+"%", userId).Scan(&numRows)
	if err != nil {
		return "", err
	}
	freshStr := "stale meme :-1:"
	if numRows == 0 {
		freshStr = "💯％ CERTIFIED ＦＲＥＳＨ 👌"
	}
	res, err := sqlClient.Exec(`insert into DiscordQuote(ChanId, AuthorId, Content, Score, IsFresh) values (?, ?, ?, ?, ?)`, chanId, userId, outStr, 0, numRows == 0)
	if err != nil {
		fmt.Println("ERROR inserting into DiscordQuote ", err.Error())
	} else {
		quoteId, err := res.LastInsertId()
		if err != nil {
			fmt.Println("ERROR getting DiscordQuote ID ", err.Error())
		} else {
			lastQuoteIds[chanId] = quoteId
			userIdUpQuotes[chanId] = make([]string, 0)
		}
	}
	return fmt.Sprintf("%s: %s\n%s", user.Username, freshStr, outStr), nil
}

func spamdiscord(session *discordgo.Session, chanId, authorId, messageId string, args []string) (string, error) {
	err := exec.Command("bash", "./gen_custom_log_by_chan.sh", chanId).Run()
	if err != nil {
		return "", err
	}
	cmd := exec.Command("/home/ross/markov/1-markov.out", "1")
	logs, err := os.Open("/home/ross/markov/chan_" + chanId + "_custom")
	if err != nil {
		return "", err
	}
	cmd.Stdin = logs
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	outStr := strings.TrimSpace(string(out))
	if match := regexp.MustCompile(`^(.*) ([[:punct:]])$`).FindStringSubmatch(outStr); match != nil {
		outStr = match[1] + match[2]
	}
	var numRows int64
	err = sqlClient.QueryRow(`select Count(Id) from Message where Content like ? and ChanId = ? and AuthorId != ?;`, "%"+outStr+"%", chanId, ownUserId).Scan(&numRows)
	if err != nil {
		return "", err
	}
	freshStr := "stale meme :-1:"
	if numRows == 0 {
		freshStr = "💯％ CERTIFIED ＦＲＥＳＨ 👌"
	}
	res, err := sqlClient.Exec(`insert into DiscordQuote(ChanId, AuthorId, Content, Score, IsFresh) values (?, ?, ?, ?, ?)`, chanId, nil, outStr, 0, numRows == 0)
	if err != nil {
		fmt.Println("ERROR inserting into DiscordQuote ", err.Error())
	} else {
		quoteId, err := res.LastInsertId()
		if err != nil {
			fmt.Println("ERROR getting DiscordQuote ID ", err.Error())
		} else {
			lastQuoteIds[chanId] = quoteId
			userIdUpQuotes[chanId] = make([]string, 0)
		}
	}
	return fmt.Sprintf("%s\n%s", freshStr, outStr), nil
}

func maths(session *discordgo.Session, chanId, authorId, messageId string, args []string) (string, error) {
	if len(args) < 1 {
		return "", errors.New("Can't do math without maths")
	}
	formula := strings.Join(args, " ")
	res, err := http.Get(fmt.Sprintf("http://api.wolframalpha.com/v2/query?input=%s&appid=%s&format=plaintext", url.QueryEscape(formula), url.QueryEscape(WOLFRAM_APPID)))
	if err != nil {
		return "", err
	}
	body, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		return "", err
	}
	var response WolframQueryResult
	err = xml.Unmarshal(body, &response)
	if err != nil {
		return "", err
	}
	if response.NumPods == len(response.Pods) && response.NumPods > 0 {
		for _, pod := range response.Pods {
			if pod.Primary != nil && *(pod.Primary) == true {
				return pod.Plaintext, nil
			}
		}
	}
	return "", errors.New("No suitable answer found")
}

func temp(session *discordgo.Session, chanId, authorId, messageId string, args []string) (string, error) {
	output, err := exec.Command("sensors", "-f", "coretemp-isa-0000").Output()
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(output), "\n")
	return fmt.Sprintf("```%s```", strings.Join(lines[2:], "\n")), nil
}

func ayy(session *discordgo.Session, chanId, authorId, messageId string, args []string) (string, error) {
	return "lmao", nil
}

func ping(session *discordgo.Session, chanId, authorId, messageId string, args []string) (string, error) {
	output, err := exec.Command("ping", "-qc3", "discordapp.com").Output()
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(output), "\n")
	return fmt.Sprintf("```%s```", strings.Join(lines[len(lines)-3:], "\n")), nil
}

func xd(session *discordgo.Session, chanId, authorId, messageId string, args []string) (string, error) {
	return "PUCK FALMER", nil
}

func asuh(session *discordgo.Session, chanId, authorId, messageId string, args []string) (string, error) {
	voiceMutex.Lock()
	defer voiceMutex.Unlock()

	channel, err := session.State.Channel(chanId)
	if err != nil {
		return "", err
	}
	guild, err := session.State.Guild(channel.GuildID)
	if err != nil {
		return "", err
	}
	voiceChanId := ""
	for _, state := range guild.VoiceStates {
		if state.UserID == authorId {
			voiceChanId = state.ChannelID
			break
		}
	}
	if voiceChanId == "" {
		return "I can't find which voice channel you're in.", nil
	}

	if currentVoiceSession != nil {
		if currentVoiceSession.ChannelID == voiceChanId && currentVoiceSession.GuildID == guild.ID {
			return "", nil
		}
		currentVoiceSession.Close()
		err = currentVoiceSession.Disconnect()
		currentVoiceSession = nil
		if err != nil {
			return "", err
		}
		time.Sleep(300 * time.Millisecond)
	}

	currentVoiceSession, err = session.ChannelVoiceJoin(guild.ID, voiceChanId, false, false)
	if err != nil {
		currentVoiceSession = nil
		return "", err
	}
	if currentVoiceTimer != nil {
		currentVoiceTimer.Stop()
	}
	currentVoiceTimer = time.AfterFunc(1*time.Minute, func() {
		if currentVoiceSession != nil {
			currentVoiceSession.Close()
			err := currentVoiceSession.Disconnect()
			currentVoiceSession = nil
			if err != nil {
				fmt.Println("ERROR disconnecting from voice channel " + err.Error())
			}
		}
	})

	time.Sleep(1 * time.Second)
	for i := 0; i < 10; i++ {
		if currentVoiceSession.Ready == false || currentVoiceSession.OpusSend == nil {
			time.Sleep(1 * time.Second)
			continue
		}
		suh := Rand.Intn(14)
		if err != nil {
			return "", err
		}
		dgvoice.PlayAudioFile(currentVoiceSession, fmt.Sprintf("suh%d.mp3", suh))
		break
	}
	return "", nil
}

func upquote(session *discordgo.Session, chanId, authorId, messageId string, args []string) (string, error) {
	lastQuoteId, found := lastQuoteIds[chanId]
	if !found {
		return "I can't find what I spammed last.", nil
	}
	for _, userId := range userIdUpQuotes[chanId] {
		if userId == authorId {
			return "You've already upquoted my last spam", nil
		}
	}
	_, err := sqlClient.Exec(`update DiscordQuote set Score = Score + 1 WHERE Id = ?`, lastQuoteId)
	if err != nil {
		return "", err
	}
	userIdUpQuotes[chanId] = append(userIdUpQuotes[chanId], authorId)
	return "", nil
}

func topquote(session *discordgo.Session, chanId, authorId, messageId string, args []string) (string, error) {
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
	rows, err := sqlClient.Query(`select AuthorId, Content, Score from DiscordQuote where ChanId = ? and Score > 0 order by Score desc limit ?`, chanId, limit)
	if err != nil {
		return "", err
	}
	channel, err := session.State.Channel(chanId)
	if err != nil {
		return "", err
	}
	messages := make([]string, limit)
	var i int
	for i = 0; rows.Next(); i++ {
		var authorId sql.NullString
		var content string
		var score int
		err = rows.Scan(&authorId, &content, &score)
		if err != nil {
			return "", err
		}
		authorName := `#` + channel.Name
		if authorId.Valid {
			author, err := session.User(authorId.String)
			if err != nil {
				return "", err
			}
			authorName = author.Username
		}
		messages[i] = fmt.Sprintf("%s (%d): %s", authorName, score, content)
	}
	return strings.Join(messages[:i], "\n"), nil
}

func eightball(session *discordgo.Session, chanId, authorId, messageId string, args []string) (string, error) {
	responses := []string{"It is certain", "It is decidedly so", "Without a doubt", "Yes, definitely", "You may rely on it", "As I see it, yes", "Most likely", "Outlook good", "Yes", "Signs point to yes", "Reply hazy try again", "Ask again later", "Better not tell you now", "Cannot predict now", "Concentrate and ask again", "Don't count on it", "My reply is no", "My sources say no", "Outlook not so good", "Very doubtful"}
	return responses[Rand.Intn(len(responses))], nil
}

func wlist(session *discordgo.Session, chanId, authorId, messageId string, args []string) (string, error) {
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
	words := WL_WORDS
	rows, err := sqlClient.Query(`select AuthorId, Content from Message where ChanId = ? and Content not like '/%'`, chanId)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	countMap := make(map[string]int64)
	for rows.Next() {
		var authorId, message string
		err := rows.Scan(&authorId, &message)
		if err != nil {
			return "", err
		}
		messageWords := strings.Fields(message)
		for i, word := range messageWords {
			_, found := words[word]
			if found {
				countMap[authorId] += 1
				continue
			}
			if i+2 > len(messageWords) {
				continue
			}
			_, found = words[strings.Join(messageWords[i:i+2], " ")]
			if found {
				countMap[authorId] += 1
				continue
			}
			if i+3 > len(messageWords) {
				continue
			}
			_, found = words[strings.Join(messageWords[i:i+3], " ")]
			if found {
				countMap[authorId] += 1
				continue
			}
			if i+4 > len(messageWords) {
				continue
			}
			_, found = words[strings.Join(messageWords[i:i+4], " ")]
			if found {
				countMap[authorId] += 1
				continue
			}
		}
	}
	var counts UserMessageLengths
	for authorId, score := range countMap {
		var numMessages int64
		err := sqlClient.QueryRow(`select count(Id) from Message where ChanId = ? and AuthorId = ? and Content not like '/%'`, chanId, authorId).Scan(&numMessages)
		if err != nil {
			return "", err
		}
		counts = append(counts, UserMessageLength{authorId, float64(score) / float64(numMessages)})
	}
	if len(counts) == 0 {
		return "You're all clean!", nil
	}
	sort.Sort(&counts)
	length := limit
	if len(counts) < limit {
		length = len(counts)
	}
	output := make([]string, length)
	for i := 0; i < length; i++ {
		author, err := session.User(counts[i].AuthorId)
		if err != nil {
			return "", err
		}
		output[i] = fmt.Sprintf("%s: %.4f", author.Username, counts[i].AvgLength)
	}
	return strings.Join(output, ", "), nil
}

func help(session *discordgo.Session, chanId, authorId, messageId string, args []string) (string, error) {
	privateChannel, err := session.UserChannelCreate(authorId)
	if err != nil {
		return "", err
	}
	_, err = session.ChannelMessageSend(privateChannel.ID, `**asuh** - joins your voice channel
**ayy**
**cputemp** - displays CPU temperature
**cwc** - alias for /spam cwc2016
**delete** - deletes last message sent by bot (if you caused it)
**downvote** [@user] - downvotes user
**@[user]--** - downvotes user
**forsen** - alias for /spam forsenlol
**karma** [number (optional)] - displays top <number> users and their karma
**lastseen** [username] - displays when <username> was last seen
**lirik** - alias for /spam lirik
**math** [math stuff] - does math
**ping** - displays ping to discordapp.com
**rename** [new username] - renames bot
**roll** [sides (optional)] - "rolls" a die with <sides> sides
**spam** [streamer (optional)] - generates a messages based on logs from <steamer>, shows all streamer logs if no streamer is specified
**spamdiscord** - generates a message based on logs from this discord channel
**spamuser** [username] - generates a message based on discord logs of <username>
**soda** - alias for /spam sodapoppin
**top** [number (optional)] - displays top <number> users sorted by messages sent
**topLength** [number (optional)] - dispalys top <number> users sorted by average words/message
**topQuote** [number (optional)] - dispalys top <number> of "quotes" from bot spam, sorted by votes from /upquote
**twitch** [channel] - displays info about twitch channel
**uptime** - displays server uptime and load
**upquote** - upvotes last statement generated by /spamuser or /spamdiscord
**uq** - alias for /upquote
**upvote** [@user] - upvotes user
**@[user]++** - upvotes user
**votes** [number (optional)] - displays top <number> users and their karma
`+string([]byte{42, 42, 119, 97, 116, 99, 104, 108, 105, 115, 116, 42, 42, 32, 91, 110, 117, 109, 98, 101, 114, 32, 40, 111, 112, 116, 105, 111, 110, 97, 108, 41, 93, 32, 45, 32, 100, 105, 115, 112, 108, 97, 121, 115, 32, 116, 111, 112, 32, 60, 110, 117, 109, 98, 101, 114, 62, 32, 117, 115, 101, 114, 115, 32, 115, 111, 114, 116, 101, 100, 32, 98, 121, 32, 116, 101, 114, 114, 111, 114, 105, 115, 109, 32, 112, 101, 114, 32, 109, 101, 115, 115, 97, 103, 101})+`
**xd**`)
	if err != nil {
		return "", err
	}
	return "", nil
}

func makeMessageCreate() func(*discordgo.Session, *discordgo.MessageCreate) {
	regexes := []*regexp.Regexp{regexp.MustCompile(`^<@` + ownUserId + `>\s+(.+)`), regexp.MustCompile(`^\/(.+)`)}
	upvoteRegex := regexp.MustCompile(`(<@\d+?>)\s*\+\+`)
	downvoteRegex := regexp.MustCompile(`(<@\d+?>)\s*--`)
	twitchRegex := regexp.MustCompile(`https?:\/\/(www.)?twitch.tv\/([[:alnum:]_]+)`)
	meanRegexes := []*regexp.Regexp{regexp.MustCompile(`fuc.*bot($|[[:space:]])`), regexp.MustCompile(`shit.*bot($|[[:space:]])`)}
	questionRegex := regexp.MustCompile(`^<@` + ownUserId + `>.*\?$`)
	inTheChatRegex := regexp.MustCompile(`(?i)can i get a\s+(.*?)\s+in the chat`)
	funcMap := map[string]Command{
		"spam":        Command(spam),
		"soda":        Command(soda),
		"lirik":       Command(lirik),
		"forsen":      Command(forsen),
		"roll":        Command(roll),
		"help":        Command(help),
		"upvote":      Command(upvote),
		"downvote":    Command(downvote),
		"votes":       Command(votes),
		"karma":       Command(votes),
		"uptime":      Command(uptime),
		"twitch":      Command(twitch),
		"top":         Command(top),
		"toplength":   Command(topLength),
		"rename":      Command(rename),
		"lastseen":    Command(lastseen),
		"delete":      Command(deleteLastMessage),
		"cwc":         Command(cwc),
		"kickme":      Command(kickme),
		"spamuser":    Command(spamuser),
		"math":        Command(maths),
		"cputemp":     Command(temp),
		"ayy":         Command(ayy),
		"spamdiscord": Command(spamdiscord),
		"ping":        Command(ping),
		"xd":          Command(xd),
		"asuh":        Command(asuh),
		"upquote":     Command(upquote),
		"uq":          Command(upquote),
		"topquote":    Command(topquote),
		"8ball":       Command(eightball),
		string([]byte{119, 97, 116, 99, 104, 108, 105, 115, 116}): Command(wlist),
	}

	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		now := time.Now()
		fmt.Printf("%20s %20s %20s > %s\n", m.ChannelID, now.Format(time.Stamp), m.Author.Username, m.Content)

		messageId, err := strconv.ParseUint(m.ID, 10, 64)
		if err != nil {
			fmt.Println("ERROR parsing message ID " + err.Error())
			return
		}
		_, err = sqlClient.Exec("INSERT INTO Message (Id, ChanId, AuthorId, Timestamp, Content) values (?, ?, ?, ?, ?)",
			messageId, m.ChannelID, m.Author.ID, now.Format(time.RFC3339Nano), m.Content)
		if err != nil {
			fmt.Println("ERROR inserting into Message")
			fmt.Println(err.Error())
		}

		if m.Author.ID == ownUserId {
			return
		}

		if typingTimer, valid := typingTimer[m.Author.ID]; valid {
			typingTimer.Stop()
		}

		if strings.Contains(strings.ToLower(m.Content), "vape") || strings.Contains(strings.ToLower(m.Content), "v/\\") || strings.Contains(strings.ToLower(m.Content), "\\//\\") || strings.Contains(strings.ToLower(m.Content), "\\\\//\\") {
			s.ChannelMessageSend(m.ChannelID, "🆅🅰🅿🅴 🅽🅰🆃🅸🅾🅽")
		}
		for _, meanRegex := range meanRegexes {
			if match := meanRegex.FindString(m.Content); match != "" {
				respond := Rand.Intn(3)
				if respond == 0 {
					responses := []string{":(", "ayy fuck you too", "asshole.", "<@" + m.Author.ID + "> --"}
					_, err := s.ChannelMessageSend(m.ChannelID, responses[Rand.Intn(len(responses))])
					if err != nil {
						fmt.Println("Error sending response " + err.Error())
					}
					break
				}
			}
		}

		var command []string
		if match := questionRegex.FindString(m.Content); match != "" {
			command = []string{"8ball"}
		}
		if match := inTheChatRegex.FindStringSubmatch(m.Content); match != nil {
			s.ChannelMessageSend(m.ChannelID, match[1])
		}
		if len(command) == 0 {
			if match := upvoteRegex.FindStringSubmatch(m.Content); match != nil {
				command = []string{"upvote", match[1]}
			}
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
			if command[0] != "upvote" &&
				command[0] != "downvote" &&
				command[0] != "help" &&
				command[0] != "rename" &&
				command[0] != "delete" &&
				command[0] != "asuh" &&
				command[0] != "uq" &&
				command[0] != "upquote" {
				s.ChannelTyping(m.ChannelID)
			}
			reply, err := cmd(s, m.ChannelID, m.Author.ID, m.ID, command[1:])
			if err != nil {
				message, msgErr := s.ChannelMessageSend(m.ChannelID, "⚠ `"+err.Error()+"`")
				if msgErr != nil {
					fmt.Println("ERROR SENDING ERROR MSG " + err.Error())
				} else {
					lastMessage = *message
					lastAuthorId = m.Author.ID
				}
				fmt.Println("ERROR in " + command[0])
				fmt.Printf("ARGS: %v\n", command[1:])
				fmt.Println("ERROR: " + err.Error())
				return
			}
			if len(reply) > 0 {
				message, err := s.ChannelMessageSend(m.ChannelID, reply)
				if err != nil {
					fmt.Println("ERROR sending message: " + err.Error())
					time.Sleep(500 * time.Millisecond)
					message, err = s.ChannelMessageSend(m.ChannelID, reply)
					if err != nil {
						fmt.Println("ERROR sending again ", err.Error())
						message, err = s.ChannelMessageSend(m.ChannelID, "⚠ `"+err.Error()+"`")
						if err != nil {
							fmt.Println("ERROR sending error")
						}
					}
				}
				lastMessage = *message
				lastAuthorId = m.Author.ID
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
				changeGame := Rand.Intn(3)
				if changeGame != 0 {
					continue
				}
				currentGame = ""
			} else {
				index := Rand.Intn(len(games) * 5)
				if index >= len(games) {
					currentGame = ""
				} else {
					currentGame = games[index]
				}
			}
			err := s.UpdateStatus(0, currentGame)
			if err != nil {
				fmt.Println("ERROR updating game: ", err.Error())
			}
		}
	}
}

func handlePresenceUpdate(s *discordgo.Session, p *discordgo.PresenceUpdate) {
	now := time.Now()
	if p.User == nil {
		return
	}
	gameName := ""
	if p.Game != nil {
		gameName = p.Game.Name
	}
	/*user, err := s.User(p.User.ID)
	if err != nil {
		fmt.Println("ERROR getting user")
		fmt.Println(err.Error())
	} else {
		fmt.Printf("%20s %20s %20s : %s %s\n", p.GuildID, now.Format(time.Stamp), user.Username, p.Status, gameName)
	}*/
	_, err := sqlClient.Exec("INSERT INTO UserPresence (GuildId, UserId, Timestamp, Presence, Game) values (?, ?, ?, ?, ?)", p.GuildID, p.User.ID, now.Format(time.RFC3339Nano), p.Status, gameName)
	if err != nil {
		fmt.Println("ERROR insert into UserPresence DB")
		fmt.Println(err.Error())
	}
}

func handleTypingStart(s *discordgo.Session, t *discordgo.TypingStart) {
	if t.UserID == ownUserId {
		return
	}
	if _, timerExists := typingTimer[t.UserID]; !timerExists && Rand.Intn(20) == 0 {
		typingTimer[t.UserID] = time.AfterFunc(20*time.Second, func() {
			responses := []string{"Something to say?", "Yes?", "Don't leave us hanging...", "I'm listening."}
			responseId := Rand.Intn(len(responses))
			s.ChannelMessageSend(t.ChannelID, fmt.Sprintf("<@%s> %s", t.UserID, responses[responseId]))
		})
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
	client.StateEnabled = true

	self, err := client.User("@me")
	if err != nil {
		fmt.Println(err)
		return
	}
	ownUserId = self.ID

	client.AddHandler(makeMessageCreate())
	client.AddHandler(handlePresenceUpdate)
	client.AddHandler(handleTypingStart)
	client.Open()
	defer client.Close()
	defer client.Logout()
	defer func() {
		voiceMutex.Lock()
		defer voiceMutex.Unlock()
		if currentVoiceSession != nil {
			currentVoiceSession.Close()
			err := currentVoiceSession.Disconnect()
			if err != nil {
				fmt.Println("ERROR leaving voice channel " + err.Error())
			}
		}
	}()

	signals := make(chan os.Signal, 1)

	go func() {
		select {
		case <-signals:
			voiceMutex.Lock()
			defer voiceMutex.Unlock()
			if currentVoiceSession != nil {
				currentVoiceSession.Close()
				err := currentVoiceSession.Disconnect()
				if err != nil {
					fmt.Println("ERROR leaving voice channel " + err.Error())
				}
			}
			client.Logout()
			client.Close()
			os.Exit(0)
		}
	}()
	signal.Notify(signals, os.Interrupt)

	gameTicker := time.NewTicker(817 * time.Second)
	go gameUpdater(client, gameTicker.C)

	var input string
	fmt.Scanln(&input)
	return
}

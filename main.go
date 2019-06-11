package main

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
	flag "github.com/spf13/pflag"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

var weekDays = [8]string{
	"all days", "monday", "thursday", "wednesday", "tuesday", "friday", "saturday", "sunday",
}
var bot *tgbotapi.BotAPI

/*

Commands:
 /add gallines 0 20:45 Cal tancar les gallines
 /add vidre 2 21:30 Cal tirar el vidre
 /noisy gallines 10
 /noisy vidre 0
 /del gallines
 /list
 /stop gallines

DataBase:
 hour/weekDay/chatID = key1/noisy_period_minutes,key2/noisy_period_minutes
 key1 = message

 20:15/0/34515 = gallines/60,vidre/0
 gallines/34515 = "Tancar galliner"
 vindre/34515 = "Cal tirar el vidre"
*/

var db *leveldb.DB

var activeEvents []string

func dbAdd(k, v string) error {
	log.Printf("Added %s = %s", k, v)
	err := db.Put([]byte(k), []byte(v), nil)
	return err
}

func dbGet(k string) string {
	data, err := db.Get([]byte(k), nil)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%s", data)
}

func dbDel(k string) error {
	err := db.Delete([]byte(k), nil)
	return err
}

func dbDump() {
	iter := db.NewIterator(nil, nil)
	for iter.Next() {
		k := iter.Key()
		fmt.Printf("%s\n", k)
	}
	iter.Release()
}

func dbListPrefix(prefix string) ([]string, error) {
	var err error
	var keys []string
	iter := db.NewIterator(util.BytesPrefix([]byte(prefix)), nil)
	for iter.Next() {
		keys = append(keys, fmt.Sprintf("%s", iter.Key()))
	}
	iter.Release()
	err = iter.Error()
	if err != nil {
		return nil, err
	}
	return keys, err
}

func list(chatID int64, humanRedeable bool) ([]string, error) {
	var err error
	var events []string
	iter := db.NewIterator(nil, nil)

	for iter.Next() {
		fields := strings.Split(fmt.Sprintf("%s", iter.Key()), "/")
		if len(fields) < 3 {
			continue
		}
		if fields[len(fields)-1] == fmt.Sprintf("%d", chatID) {
			hour := fields[0]
			wday, _ := strconv.Atoi(fields[1])
			for _, eventAndPeriod := range strings.Split(fmt.Sprintf("%s", iter.Value()), ",") {
				event := strings.Split(fmt.Sprintf("%s", eventAndPeriod), "/")
				noisy := "0"
				if len(event) > 1 {
					noisy = event[1]
				}
				name := event[0]
				if humanRedeable {
					events = append(events, fmt.Sprintf("\n\r[%s] Hour %s, %s, Noisy %s", name, hour, weekDays[wday], noisy))
				} else {
					events = append(events, fmt.Sprintf("%s", iter.Key()))
				}
			}
		}
	}
	iter.Release()
	err = iter.Error()
	if err != nil {
		return nil, err
	}
	return events, err
}

func addEvent(chatid int64, event, repeat, weekDay, hour string, msg []string) error {
	wday, err := strconv.Atoi(weekDay)
	if err != nil {
		return errors.New("Week day is not a number")
	}
	if wday > 7 {
		return errors.New("Week day is not between 0 and 7")
	}

	t, err := time.Parse("15:04", hour)
	if err != nil {
		return errors.New("Hour is not in valid format, use HH:MM (24h)")
	}

	repTime, err := strconv.Atoi(repeat)
	if err != nil {
		return errors.New("Noisy time wrong format, expecting minutes")
	}
	if repTime > 1440 {
		return errors.New("Noisy time too big")
	}

	event = strings.ReplaceAll(strings.TrimSpace(event), "/", "")

	key := fmt.Sprintf("%d:%d/%d/%d", t.Hour(), t.Minute(), wday, chatid)
	newValue := fmt.Sprintf("%s/%d", event, repTime)

	current := dbGet(key) // current = name1/noisy1,name2/noisy2,...

	// if there is another event for the same hour and chatID, let's concat it
	if len(current) > 0 {
		currentArray := strings.Split(current, ",")
		for _, c := range currentArray {
			if strings.Split(c, "/")[0] == event {
				continue
			}
			newValue = fmt.Sprintf("%s,%s", newValue, c)
		}
	}

	err = dbAdd(key, newValue)
	if err != nil {
		return err
	}

	// add message db entry
	key = fmt.Sprintf("%s/%d", event, chatid)
	err = dbAdd(key, strings.Join(msg, " "))

	return err
}

func addNoisy(chatID int64, event, period string) error {
	p, err := strconv.Atoi(period)
	if err != nil {
		return errors.New("Noisy period is not a number")
	}
	if p > 1440 {
		return errors.New("Noisy period too big")
	}
	chatKeys, err := list(chatID, false)
	if err != nil {
		return err
	}
	for _, k := range chatKeys {
		v := dbGet(k)
		if len(v) == 0 {
			continue
		}
		newEvent := ""
		found := false
		eventList := strings.Split(v, ",")
		for i, er := range eventList {
			e := strings.Split(er, "/")
			if e[0] == event {
				log.Printf("Updating noisy of %s to %s", event, period)
				if len(eventList) == i+1 {
					newEvent = fmt.Sprintf("%s%s/%s", newEvent, event, period)
				} else {
					newEvent = fmt.Sprintf("%s%s/%s,", newEvent, event, period)
				}
				found = true
			} else {
				if len(eventList) == i+1 {
					newEvent = fmt.Sprintf("%s%s", newEvent, er)
				} else {
					newEvent = fmt.Sprintf("%s%s,", newEvent, er)
				}
			}
		}
		if found {
			err = dbAdd(k, newEvent)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func contains(a []string, x string) bool {
	for _, n := range a {
		if x == n {
			return true
		}
	}
	return false
}

func remove(s []string, r string) []string {
	for i, v := range s {
		if v == r {
			return append(s[:i], s[i+1:]...)
		}
	}
	return s
}

func sendMsg(chatID int, event, msg string, repeat int) {
	key := fmt.Sprintf("%s/%d", event, chatID)
	activeEvents = append(activeEvents, key)
	for {
		m := tgbotapi.NewMessage(int64(chatID), msg)
		log.Printf("Sending message to %d: %s", chatID, msg)
		bot.Send(m)
		if repeat == 0 {
			break
		}
		time.Sleep(time.Minute * time.Duration(repeat))
		if !contains(activeEvents, key) {
			break
		}
	}
	log.Printf("End of event %s", event)
	activeEvents = remove(activeEvents, key)
}

func eventHandler(chatID int, data string) {
	for _, eventKey := range strings.Split(data, ",") {
		eventKeyArray := strings.Split(eventKey, "/")
		key := eventKeyArray[0]
		repeat, err := strconv.Atoi(eventKeyArray[1])
		if err != nil {
			continue
		}
		msg := dbGet(fmt.Sprintf("%s/%d", key, chatID))
		if len(msg) < 1 {
			log.Printf("Key nout found %s/%d", key, chatID)
			continue
		}
		go sendMsg(chatID, key, msg, repeat)
	}
}

func timerCheck() {
	for {
		ct := time.Now()
		weekday := (int(ct.Weekday()+6) % 7) + 1 // monday = 1, sunday = 7
		prefix := fmt.Sprintf("%d:%d", ct.Hour(), ct.Minute())
		eventList, err := dbListPrefix(prefix)
		if err != nil {
			log.Printf(err.Error())
			continue
		}
		for _, event := range eventList {
			eventArray := strings.Split(event, "/")
			if len(eventArray) < 3 {
				continue
			}
			eventWeekDay, err := strconv.Atoi(eventArray[1])
			if err != nil || eventWeekDay > 7 {
				continue
			}
			if eventWeekDay != 0 && eventWeekDay != weekday {
				continue
			}
			chatID, err := strconv.Atoi(eventArray[2])
			if err != nil {
				continue
			}
			data := dbGet(event)
			log.Printf("Parsing event %s", event)
			go eventHandler(chatID, data)
		}
		time.Sleep(60 * time.Second)
	}
}

var keyboardMain = tgbotapi.NewInlineKeyboardMarkup(
	tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("add", "add"),
		tgbotapi.NewInlineKeyboardButtonData("list", "list"),
		tgbotapi.NewInlineKeyboardButtonData("noisy", "noisy"),
	),
	tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("stop", "stop"),
		tgbotapi.NewInlineKeyboardButtonData("del", "del"),
		tgbotapi.NewInlineKeyboardButtonData("close", "close"),
	),
)

var keyboardNum = tgbotapi.NewInlineKeyboardMarkup(
	tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("1", "1"),
		tgbotapi.NewInlineKeyboardButtonData("2", "2"),
		tgbotapi.NewInlineKeyboardButtonData("3", "3"),
		tgbotapi.NewInlineKeyboardButtonData("4", "4"),
		tgbotapi.NewInlineKeyboardButtonData("5", "5"),
	),
	tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("6", "6"),
		tgbotapi.NewInlineKeyboardButtonData("7", "7"),
		tgbotapi.NewInlineKeyboardButtonData("8", "8"),
		tgbotapi.NewInlineKeyboardButtonData("9", "9"),
		tgbotapi.NewInlineKeyboardButtonData("0", "0"),
	),
)

func main() {
	var err error
	telegramToken := flag.String("token", "", "BotFather telegram token")
	flag.Parse()

	if len(*telegramToken) < 10 {
		log.Fatal("Telegram token must be specified")
	}

	db, err = leveldb.OpenFile("db.cuckoo", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	fmt.Println("--------database---------")
	dbDump()
	fmt.Println("-------------------------")

	bot, err = tgbotapi.NewBotAPI(*telegramToken)
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = false

	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := bot.GetUpdatesChan(u)
	go timerCheck()
	for update := range updates {
		if update.CallbackQuery != nil {
			fmt.Println(update.CallbackQuery.Data)
			if update.CallbackQuery.Data == "add" {
				//bot.AnswerCallbackQuery(tgbotapi.NewCallback(update.CallbackQuery.ID, update.CallbackQuery.Data))
				msg := tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, "Choose hour")
				msg.ReplyMarkup = keyboardNum
				bot.Send(msg)
				log.Println("Enable numeric keyboard")
			} else {
				bot.AnswerCallbackQuery(tgbotapi.NewCallback(update.CallbackQuery.ID, update.CallbackQuery.Data))
				bot.Send(tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Data))
			}
			continue
		}

		if update.Message == nil || !update.Message.IsCommand() {
			continue
		}

		username := update.Message.From.UserName
		chatID := update.Message.Chat.ID

		cmd := update.Message.Command()
		text := update.Message.CommandArguments()
		openKeyboard := false
		reply := "ok!"
		switch cmd {
		case "add":
			log.Printf("Received /add from %s", username)
			args := strings.Split(text, " ")
			if len(args) < 4 {
				reply = "No enough arguments for /add"
				break
			}
			err = addEvent(chatID, args[0], "0", args[1], args[2], args[3:])
			if err != nil {
				reply = err.Error()
			}
			break
		case "list":
			l, err := list(chatID, true)
			if err != nil {
				reply = err.Error()
				break
			}
			reply = strings.Join(l, " ")
			break
		case "noisy":
			args := strings.Split(text, " ")
			if len(args) < 2 {
				reply = "Not enough arguments for /noisy"
				break
			}
			err = addNoisy(chatID, args[0], args[1])
			if err != nil {
				reply = err.Error()
			}
		case "stop":
			args := strings.Split(text, " ")
			if len(args) < 1 {
				reply = "Not enough arguments for /stop"
				break
			}
			key := fmt.Sprintf("%s/%d", args[0], chatID)
			log.Printf("Stop noisy %s", key)
			activeEvents = remove(activeEvents, key)
			break
		case "open":
			openKeyboard = true
			break
		case "close":
			openKeyboard = false
			break
		default:
			reply = "I don't understand"
		}
		msg := tgbotapi.NewMessage(chatID, reply)
		if openKeyboard {
			msg.ReplyMarkup = keyboardMain
		}
		msg.ReplyToMessageID = update.Message.MessageID

		bot.Send(msg)
	}
}

package notifiers

import (
	. "Notifier/models"
	. "Notifier/src/utils"
	"cloud.google.com/go/firestore"
	"context"
	"errors"
	"github.com/PuerkitoBio/goquery"
	"github.com/slack-go/slack"
	"net/http"
	"os"
	"strconv"
	"strings"
)

type Type5Notifier BaseNotifier

func (Type5Notifier) New(config NotifierConfig) *Type5Notifier {
	dbData := LoadDbData(config.DocumentID)

	return &Type5Notifier{
		URL:               config.URL,
		Source:            config.Source,
		ChannelID:         config.ChannelID,
		DocumentID:        config.DocumentID,
		BoxCount:          int(dbData["box"].(int64)),
		MaxNum:            int(dbData["num"].(int64)),
		BoxNoticeSelector: "#nil",
		NumNoticeSelector: "#contents > article > section > div > div.tb_w > table > tbody > tr",
	}
}

func (notifier *Type5Notifier) Notify() {
	defer func() {
		recover()
	}()

	notices := notifier.scrapeNotice()
	for _, notice := range notices {
		notifier.sendNoticeToSlack(notice)
	}
}

func (notifier *Type5Notifier) scrapeNotice() []Notice {
	resp, err := http.Get(notifier.URL)
	if err != nil {
		ErrorLogger.Panic(err)
	}
	if resp.StatusCode != http.StatusOK {
		ErrorLogger.Panicf("status code error: %s", resp.Status)
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		ErrorLogger.Panic(err)
	}

	err = notifier.checkHTML(doc)
	if err != nil {
		ErrorLogger.Panic(err)
	}

	boxNotices := notifier.scrapeBoxNotice(doc)

	numNotices := notifier.scrapeNumNotice(doc)

	notices := make([]Notice, 0, len(boxNotices)+len(numNotices))
	for _, notice := range boxNotices {
		notices = append(notices, notice)
	}
	for _, notice := range numNotices {
		notices = append(notices, notice)
	}

	for _, notice := range notices {
		SentNoticeLogger.Println("notice =>", notice)
	}

	return notices
}

func (notifier *Type5Notifier) checkHTML(doc *goquery.Document) error {
	if notifier.isInvalidHTML(doc) {
		errMsg := strings.Join([]string{"HTML structure has changed at ", notifier.Source}, "")
		return errors.New(errMsg)
	}
	return nil
}

func (notifier *Type5Notifier) isInvalidHTML(doc *goquery.Document) bool {
	sel := doc.Find(notifier.NumNoticeSelector)
	if sel.Nodes == nil ||
		sel.Find("td:nth-child(1)").Nodes == nil ||
		sel.Find("td:nth-child(2)").Nodes == nil ||
		sel.Find("td:nth-child(3) > a").Nodes == nil ||
		sel.Find("td:nth-child(3) > a > span").Nodes == nil ||
		sel.Find("td:nth-child(6)").Nodes == nil {
		return true
	}
	return false
}

func (notifier *Type5Notifier) scrapeBoxNotice(doc *goquery.Document) []Notice {
	boxNoticeSels := doc.Find(notifier.BoxNoticeSelector)
	boxCount := boxNoticeSels.Length()

	boxNoticeChan := make(chan Notice, boxCount)
	boxNotices := make([]Notice, 0, boxCount)
	boxNoticeCount := boxCount - notifier.BoxCount

	if boxCount > notifier.BoxCount {
		boxNoticeSels = boxNoticeSels.FilterFunction(func(i int, _ *goquery.Selection) bool {
			return i < boxNoticeCount
		})

		boxNoticeSels.Each(func(_ int, boxNotice *goquery.Selection) {
			go notifier.getNotice(boxNotice, boxNoticeChan)
		})

		for i := 0; i < boxNoticeCount; i++ {
			boxNotices = append(boxNotices, <-boxNoticeChan)
		}

		notifier.BoxCount = boxCount
		_, err := Client.Collection("notice").Doc(notifier.DocumentID).Update(context.Background(), []firestore.Update{
			{
				Path:  "box",
				Value: notifier.BoxCount,
			},
		})
		if err != nil {
			ErrorLogger.Panic(err)
		}
	} else if boxCount < notifier.BoxCount {
		notifier.BoxCount = boxCount
		_, err := Client.Collection("notice").Doc(notifier.DocumentID).Update(context.Background(), []firestore.Update{
			{
				Path:  "box",
				Value: notifier.BoxCount,
			},
		})
		if err != nil {
			ErrorLogger.Panic(err)
		}
	}

	return boxNotices
}

func (notifier *Type5Notifier) scrapeNumNotice(doc *goquery.Document) []Notice {
	numNoticeSels := doc.Find(notifier.NumNoticeSelector)
	maxNumText := numNoticeSels.First().Find("td:first-child").Text()
	maxNumText = strings.TrimSpace(maxNumText)
	maxNum, err := strconv.Atoi(maxNumText)
	if err != nil {
		ErrorLogger.Panic(err)
	}

	numNoticeCount := min(maxNum-notifier.MaxNum, MaxNumNoticeCount)
	numNoticeChan := make(chan Notice, numNoticeCount)
	numNotices := make([]Notice, 0, numNoticeCount)

	if maxNum > notifier.MaxNum {
		numNoticeSels = numNoticeSels.FilterFunction(func(i int, _ *goquery.Selection) bool {
			return i < numNoticeCount
		})

		numNoticeSels.Each(func(_ int, numNotice *goquery.Selection) {
			go notifier.getNotice(numNotice, numNoticeChan)
		})

		for i := 0; i < numNoticeCount; i++ {
			numNotices = append(numNotices, <-numNoticeChan)
		}

		notifier.MaxNum = maxNum
		_, err = Client.Collection("notice").Doc(notifier.DocumentID).Update(context.Background(), []firestore.Update{
			{
				Path:  "num",
				Value: notifier.MaxNum,
			},
		})
		if err != nil {
			ErrorLogger.Panic(err)
		}
	}

	return numNotices
}

func (notifier *Type5Notifier) getNotice(sel *goquery.Selection, noticeChan chan Notice) {
	id := sel.Find("td:nth-child(1)").Text()
	id = strings.TrimSpace(id)

	category := sel.Find("td:nth-child(2)").Text()
	category = strings.TrimSpace(category)

	link, _ := sel.Find("td:nth-child(3) > a").Attr("href")
	split := strings.FieldsFunc(link, func(c rune) bool {
		return c == ' '
	})
	link = split[5]
	link = strings.Join([]string{notifier.URL[:len(notifier.URL)-7], "View.do?no=", link}, "")

	title := sel.Find("td:nth-child(3) > a > span").Text()

	date := sel.Find("td:nth-child(6)").Text()
	month := date[5:7]
	if month[0] == '0' {
		month = month[1:]
	}
	day := date[8:10]
	if day[0] == '0' {
		day = day[1:]
	}
	date = strings.Join([]string{month, "월", day, "일"}, "")

	notice := Notice{ID: id, Category: category, Title: title, Date: date, Link: link}

	noticeChan <- notice
}

func (notifier *Type5Notifier) sendNoticeToSlack(notice Notice) {
	api := slack.New(os.Getenv("SLACK_TOKEN"))

	category := strings.Join([]string{"[", notice.Category, "]"}, "")
	footer := strings.Join([]string{notifier.Source, category}, " ")

	attachment := slack.Attachment{
		Color:      "#0072ce",
		Title:      strings.Join([]string{notice.Date, notice.Title}, " "),
		Text:       notice.Link,
		Footer:     footer,
		FooterIcon: "https://github.com/zzzang12/Notifier/assets/70265177/48fd0fd7-80e2-4309-93da-8a6bc957aacf",
	}

	_, _, err := api.PostMessage(notifier.ChannelID, slack.MsgOptionAttachments(attachment))
	if err != nil {
		ErrorLogger.Panic(err)
	}
}

package notifier

import (
	"Notifier/src/models"
	. "Notifier/src/utils"
	"cloud.google.com/go/firestore"
	"context"
	"errors"
	"github.com/PuerkitoBio/goquery"
	"github.com/slack-go/slack"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/transform"
	"net/http"
	"os"
	"strconv"
	"strings"
)

type Type3Notifier models.BaseNotifier

func (Type3Notifier) New(config models.NotifierConfig) *Type3Notifier {
	fsDocID := config.FsDocID
	dsnap, err := Client.Collection("notice").Doc(fsDocID).Get(context.Background())
	if err != nil {
		ErrorLogger.Panic(err)
	}
	dbData := dsnap.Data()

	return &Type3Notifier{
		URL:               config.URL,
		Source:            config.Source,
		ChannelID:         config.ChannelID,
		FsDocID:           fsDocID,
		BoxCount:          int(dbData["box"].(int64)),
		MaxNum:            int(dbData["num"].(int64)),
		BoxNoticeSelector: "#sub_contents > div > div.conbody > table:nth-child(2) > tbody > tr:nth-child(n+4):nth-last-child(n+3):nth-of-type(2n):has(td:first-child > img)",
		NumNoticeSelector: "#sub_contents > div > div.conbody > table:nth-child(2) > tbody > tr:nth-child(n+4):nth-last-child(n+3):nth-of-type(2n):not(:has(td:first-child > img))",
	}
}

func (notifier *Type3Notifier) Notify() {
	defer func() {
		recover()
	}()

	notices := notifier.scrapeNotice()
	for _, notice := range notices {
		notifier.sendNoticeToSlack(notice)
	}
}

func (notifier *Type3Notifier) scrapeNotice() []models.Notice {
	resp, err := http.Get(notifier.URL)
	if err != nil {
		ErrorLogger.Panic(err)
	}
	if resp.StatusCode != 200 {
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

	notices := make([]models.Notice, 0, len(boxNotices)+len(numNotices))
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

func (notifier *Type3Notifier) checkHTML(doc *goquery.Document) error {
	if notifier.isInvalidHTML(doc) {
		errMsg := strings.Join([]string{"HTML structure has changed at ", notifier.Source}, "")
		return errors.New(errMsg)
	}
	return nil
}

func (notifier *Type3Notifier) isInvalidHTML(doc *goquery.Document) bool {
	sel1 := doc.Find(notifier.BoxNoticeSelector)
	sel2 := doc.Find(notifier.NumNoticeSelector)
	if sel1.Nodes == nil || sel2.Nodes == nil ||
		sel1.Find("td:nth-child(1)").Nodes == nil ||
		sel1.Find("td:nth-child(3) > a").Nodes == nil ||
		sel1.Find("td:nth-child(3) > p:first-of-type").Nodes == nil ||
		sel2.Find("td:nth-child(1)").Nodes == nil ||
		sel2.Find("td:nth-child(3) > a").Nodes == nil ||
		sel2.Find("td:nth-child(3) > p:first-of-type").Nodes == nil {
		return true
	}
	return false
}

func (notifier *Type3Notifier) scrapeBoxNotice(doc *goquery.Document) []models.Notice {
	boxNoticeSels := doc.Find(notifier.BoxNoticeSelector)
	boxCount := boxNoticeSels.Length()

	boxNoticeChan := make(chan models.Notice, boxCount)
	boxNotices := make([]models.Notice, 0, boxCount)
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
		_, err := Client.Collection("notice").Doc(notifier.FsDocID).Update(context.Background(), []firestore.Update{
			{
				Path:  "box",
				Value: notifier.BoxCount,
			},
		})
		if err != nil {
			ErrorLogger.Panic(err)
		}
		BoxCountMaxNumLogger.Println("boxCount =>", notifier.BoxCount)
	} else if boxCount < notifier.BoxCount {
		notifier.BoxCount = boxCount
		_, err := Client.Collection("notice").Doc(notifier.FsDocID).Update(context.Background(), []firestore.Update{
			{
				Path:  "box",
				Value: notifier.BoxCount,
			},
		})
		if err != nil {
			ErrorLogger.Panic(err)
		}
		BoxCountMaxNumLogger.Println("boxCount =>", notifier.BoxCount)
	}

	return boxNotices
}

func (notifier *Type3Notifier) scrapeNumNotice(doc *goquery.Document) []models.Notice {
	numNoticeSels := doc.Find(notifier.NumNoticeSelector)
	maxNumText := numNoticeSels.First().Find("td:first-child").Text()
	maxNumText = strings.TrimSpace(maxNumText)
	maxNum, err := strconv.Atoi(maxNumText)
	if err != nil {
		ErrorLogger.Panic(err)
	}

	numNoticeCount := min(maxNum-notifier.MaxNum, numNoticeSels.Length())
	numNoticeChan := make(chan models.Notice, numNoticeCount)
	numNotices := make([]models.Notice, 0, numNoticeCount)

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
		_, err = Client.Collection("notice").Doc(notifier.FsDocID).Update(context.Background(), []firestore.Update{
			{
				Path:  "num",
				Value: notifier.MaxNum,
			},
		})
		if err != nil {
			ErrorLogger.Panic(err)
		}
		BoxCountMaxNumLogger.Println("maxNum =>", notifier.MaxNum)
	}

	return numNotices
}

func (notifier *Type3Notifier) getNotice(sel *goquery.Selection, noticeChan chan models.Notice) {
	var id string
	if sel.Find("td:nth-child(1):has(img)").Nodes != nil {
		id = "공지"
	} else {
		id = sel.Find("td:nth-child(1)").Text()
		id = strings.TrimSpace(id)
	}

	title := sel.Find("td:nth-child(3) > a").Text()
	title = strings.TrimSpace(title)
	title, _, _ = transform.String(korean.EUCKR.NewDecoder(), title)

	link, _ := sel.Find("td:nth-child(3) > a").Attr("href")
	split := strings.FieldsFunc(link, func(c rune) bool {
		return c == '&'
	})
	link = strings.Join(split[1:3], "&")
	link = strings.Join([]string{notifier.URL, link}, "&")

	date := sel.Find("td:nth-child(3) > p:first-of-type").Text()
	month := date[5:7]
	if month[0] == '0' {
		month = month[1:]
	}
	day := date[8:10]
	if day[0] == '0' {
		day = day[1:]
	}
	date = strings.Join([]string{month, "월", day, "일"}, "")

	notice := models.Notice{ID: id, Title: title, Date: date, Link: link}

	noticeChan <- notice
}

func (notifier *Type3Notifier) sendNoticeToSlack(notice models.Notice) {
	api := slack.New(os.Getenv("SLACK_TOKEN"))

	var footer string
	if notice.ID == "공지" {
		footer = "[중요]"
	}
	footer = strings.Join([]string{footer, notifier.Source}, " ")

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
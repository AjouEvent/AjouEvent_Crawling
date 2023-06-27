package notifier

import (
	"Notifier/internal/utils"
	"cloud.google.com/go/firestore"
	"context"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/slack-go/slack"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

type Notice struct {
	id         string
	category   string
	title      string
	department string
	date       string
	link       string
}

var AjouNormalBoxCount int
var AjouNormalMaxNum int

func GetAjouNormalFromDB() (int, int) {
	dsnap, err := utils.Client.Collection("notice").Doc("ajouNormal").Get(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	ajouNormal := dsnap.Data()
	return int(ajouNormal["box"].(int64)), int(ajouNormal["num"].(int64))
}

func AjouNormalFunc(dbBoxCount, dbMaxNum int) {
	notices := scrapeNotice(dbBoxCount, dbMaxNum)
	for _, notice := range notices {
		sendMessageToSlack(notice)
	}
}

func sendMessageToSlack(notice Notice) {
	api := slack.New(os.Getenv("SLACK_TOKEN"))

	footer := ""
	if notice.id == "공지" {
		footer = "[중요]"
	}
	category := strings.Join([]string{"[", notice.category, "]"}, "")
	department := strings.Join([]string{"[", notice.department, "]"}, "")
	footer = strings.Join([]string{footer, category, department}, " ")

	attachment := slack.Attachment{
		Color:      "#0072ce",
		Title:      strings.Join([]string{notice.date, notice.title}, " "),
		Text:       notice.link,
		Footer:     footer,
		FooterIcon: "https://github.com/zzzang12/Notifier/assets/70265177/48fd0fd7-80e2-4309-93da-8a6bc957aacf",
	}

	_, _, err := api.PostMessage("아주대학교-공지사항", slack.MsgOptionAttachments(attachment))
	if err != nil {
		log.Fatal(err)
	}
}

func scrapeNotice(dbBoxCount, dbMaxNum int) []Notice {
	noticeURL := "https://ajou.ac.kr/kr/ajou/notice.do"

	resp, err := http.Get(noticeURL)
	if err != nil {
		log.Fatal(err)
	}
	if resp.StatusCode != 200 {
		log.Fatalf("status code error: %s", resp.Status)
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	boxNotices := scrapeBoxNotice(doc, dbBoxCount, noticeURL)

	numNotices := scrapeNumNotice(doc, dbMaxNum, noticeURL)

	notices := make([]Notice, 0, len(boxNotices)+len(numNotices))
	for _, notice := range boxNotices {
		notices = append(notices, notice)
	}
	for _, notice := range numNotices {
		notices = append(notices, notice)
	}

	for _, notice := range notices {
		fmt.Println("notice =>", notice)
	}

	return notices
}

func scrapeBoxNotice(doc *goquery.Document, dbBoxCount int, noticeURL string) []Notice {
	boxNoticeSels := doc.Find("#cms-content > div > div > div.bn-list-common02.type01.bn-common-cate > table > tbody > tr[class$=\"b-top-box\"]")
	boxCount := boxNoticeSels.Length()

	boxNoticeChan := make(chan Notice, boxCount)
	boxNotices := make([]Notice, 0, boxCount)
	boxNoticeCount := boxCount - dbBoxCount

	if boxCount > dbBoxCount {
		boxNoticeSels = boxNoticeSels.FilterFunction(func(i int, _ *goquery.Selection) bool {
			return i < boxNoticeCount
		})

		boxNoticeSels.Each(func(_ int, boxNotice *goquery.Selection) {
			go getNotice(noticeURL, boxNotice, boxNoticeChan)
		})

		for i := 0; i < boxNoticeCount; i++ {
			boxNotices = append(boxNotices, <-boxNoticeChan)
		}

		AjouNormalBoxCount = boxCount
		_, err := utils.Client.Collection("notice").Doc("ajouNormal").Update(context.Background(), []firestore.Update{
			{
				Path:  "box",
				Value: boxCount,
			},
		})
		if err != nil {
			log.Fatal(err)
		}
	} else if boxCount < dbBoxCount {
		AjouNormalBoxCount = boxCount
		_, err := utils.Client.Collection("notice").Doc("ajouNormal").Update(context.Background(), []firestore.Update{
			{
				Path:  "box",
				Value: boxCount,
			},
		})
		if err != nil {
			log.Fatal(err)
		}
	}

	return boxNotices
}

func scrapeNumNotice(doc *goquery.Document, dbMaxNum int, noticeURL string) []Notice {
	numNoticeSels := doc.Find("#cms-content > div > div > div.bn-list-common02.type01.bn-common-cate > table > tbody > tr:not([class$=\"b-top-box\"])")
	maxNumText := numNoticeSels.First().Find("td:nth-child(1)").Text()
	maxNumText = strings.TrimSpace(maxNumText)
	maxNum, err := strconv.Atoi(maxNumText)
	if err != nil {
		log.Fatal(err)
	}

	numNoticeChan := make(chan Notice, utils.MaxNumCount)
	numNotices := make([]Notice, 0, utils.MaxNumCount)
	numNoticeCount := maxNum - dbMaxNum
	numNoticeCount = utils.Min(numNoticeCount, utils.MaxNumCount)

	if maxNum > dbMaxNum {
		numNoticeSels = numNoticeSels.FilterFunction(func(i int, _ *goquery.Selection) bool {
			return i < numNoticeCount
		})

		numNoticeSels.Each(func(_ int, numNotice *goquery.Selection) {
			go getNotice(noticeURL, numNotice, numNoticeChan)
		})

		for i := 0; i < numNoticeCount; i++ {
			numNotices = append(numNotices, <-numNoticeChan)
		}

		AjouNormalMaxNum = maxNum
		_, err = utils.Client.Collection("notice").Doc("ajouNormal").Update(context.Background(), []firestore.Update{
			{
				Path:  "num",
				Value: maxNum,
			},
		})
		if err != nil {
			log.Fatal(err)
		}
	}

	return numNotices
}

func getNotice(noticeURL string, sel *goquery.Selection, noticeChan chan Notice) {
	id := sel.Find("td:nth-child(1)").Text()
	id = strings.TrimSpace(id)

	category := sel.Find("td:nth-child(2)").Text()
	category = strings.TrimSpace(category)

	title, _ := sel.Find("td:nth-child(3) > div > a").Attr("title")
	title = title[:len(title)-17]

	link, _ := sel.Find("td:nth-child(3) > div > a").Attr("href")
	split := strings.FieldsFunc(link, func(c rune) bool {
		return c == '&'
	})
	link = strings.Join(split[0:2], "&")
	link = strings.Join([]string{noticeURL, link}, "")

	department := sel.Find("td:nth-child(5)").Text()

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

	notice := Notice{id, category, title, department, date, link}

	noticeChan <- notice
}
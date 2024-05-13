package notifiers

import (
	"strings"
	"time"

	. "Notifier/models"
	. "Notifier/src/utils"
	"github.com/PuerkitoBio/goquery"
)

type Type4Notifier struct {
	BaseNotifier
}

func (Type4Notifier) New(baseNotifier *BaseNotifier) *Type4Notifier {
	baseNotifier.BoxNoticeSelector = "#nil"
	baseNotifier.NumNoticeSelector = "#contents > article > section > div > div.tb_w > table > tbody > tr"
	baseNotifier.ContentSelector = "#contents > article > section > div > div > dl > dd.board_view_txt > div.txt span"
	baseNotifier.ImagesSelector = "#contents > article > section > div > div > dl > dd.board_view_txt > div.txt img"

	return &Type4Notifier{
		BaseNotifier: *baseNotifier,
	}
}

func (notifier *Type4Notifier) isInvalidHTML(doc *goquery.Document) bool {
	sel := doc.Find(notifier.NumNoticeSelector)
	if sel.Nodes == nil ||
		sel.Find("td:nth-child(1)").Nodes == nil ||
		sel.Find("td:nth-child(2)").Nodes == nil ||
		sel.Find("td:nth-child(3) > a").Nodes == nil ||
		sel.Find("td:nth-child(3) > a > span").Nodes == nil {
		return true
	}
	return false
}

func (notifier *Type4Notifier) getNotice(sel *goquery.Selection, noticeChan chan Notice) {
	id := sel.Find("td:nth-child(1)").Text()
	id = strings.TrimSpace(id)

	category := sel.Find("td:nth-child(2)").Text()
	category = strings.TrimSpace(category)

	url, _ := sel.Find("td:nth-child(3) > a").Attr("href")
	split := strings.FieldsFunc(url, func(c rune) bool {
		return c == ' '
	})
	url = notifier.NoticeUrl[:len(notifier.NoticeUrl)-7] + "View.do?no=" + split[5]

	title := sel.Find("td:nth-child(3) > a > span").Text()
	title = strings.TrimSpace(title)

	date := time.Now().Format(time.RFC3339)
	date = date[:19]

	doc := NewDocumentFromPage(url)

	contents := make([]string, 0, sel.Length())
	sel = doc.Find(notifier.ContentSelector)
	sel.Each(func(_ int, s *goquery.Selection) {
		if s.Text() != "" && s.Text() != "\u00a0" {
			str := strings.ReplaceAll(s.Text(), "\u00a0", " ")
			str = strings.ReplaceAll(str, "\n\n", "\\n")
			str = strings.ReplaceAll(str, "\n", "\\n")
			contents = append(contents, strings.TrimSpace(str))
		}
	})
	content := strings.Join(contents, "\\n")

	images := make([]string, 0, sel.Length())
	sel = doc.Find(notifier.ImagesSelector)
	sel.Each(func(_ int, s *goquery.Selection) {
		image, _ := s.Attr("src")
		images = append(images, image)
	})

	notice := Notice{
		ID:           id,
		Category:     category,
		Title:        title,
		Date:         date,
		Url:          url,
		Content:      content,
		Images:       images,
		EnglishTopic: notifier.EnglishTopic,
		KoreanTopic:  notifier.KoreanTopic,
	}

	noticeChan <- notice
}

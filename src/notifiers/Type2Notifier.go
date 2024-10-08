package notifiers

import (
	"strings"
	"time"

	. "Notifier/models"
	. "Notifier/src/utils"
	"github.com/PuerkitoBio/goquery"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/transform"
)

type Type2Notifier struct {
	BaseNotifier
}

func (Type2Notifier) New(baseNotifier *BaseNotifier) *Type2Notifier {
	baseNotifier.BoxNoticeSelector = "#sub_contents > div > div.conbody > table:nth-child(2) > tbody > tr:nth-child(n+4):nth-last-child(n+3):nth-of-type(2n):has(td:first-child > img)"
	baseNotifier.NumNoticeSelector = "#sub_contents > div > div.conbody > table:nth-child(2) > tbody > tr:nth-child(n+4):nth-last-child(n+3):nth-of-type(2n):not(:has(td:first-child > img))"
	baseNotifier.ContentSelector = "#DivContents p"
	baseNotifier.ImagesSelector = "#DivContents img"

	return &Type2Notifier{
		BaseNotifier: *baseNotifier,
	}
}

func (notifier *Type2Notifier) isInvalidHTML(doc *goquery.Document) bool {
	sel := doc.Find(notifier.NumNoticeSelector)
	if sel.Nodes == nil ||
		sel.Find("td:nth-child(1)").Nodes == nil ||
		sel.Find("td:nth-child(3) > a").Nodes == nil {
		return true
	}
	return false
}

func (notifier *Type2Notifier) getNotice(sel *goquery.Selection, noticeChan chan Notice) {
	var id string
	if sel.Find("td:nth-child(1):has(img)").Nodes != nil {
		id = "공지"
	} else {
		id = sel.Find("td:nth-child(1)").Text()
		id = strings.TrimSpace(id)
	}

	title := sel.Find("td:nth-child(3) > a").Text()
	title, _, _ = transform.String(korean.EUCKR.NewDecoder(), title)
	title = strings.TrimSpace(title)

	url, _ := sel.Find("td:nth-child(3) > a").Attr("href")
	split := strings.FieldsFunc(url, func(c rune) bool {
		return c == '&'
	})
	url = notifier.NoticeUrl + "&" + strings.Join(split[1:3], "&")

	department := sel.Find("td:nth-child(5)").Text()
	department, _, _ = transform.String(korean.EUCKR.NewDecoder(), department)
	department = strings.TrimSpace(department)

	date := time.Now().Format(time.RFC3339)
	date = date[:19]

	doc, err := NewDocumentFromPage(url)
    if err != nil {
        ErrorLogger.Printf("Failed to load notice page: %s, URL: %s", err, url)
        noticeChan <- Notice{}  // 에러 발생 시 빈 Notice 반환 
        return
    }

	contents := make([]string, 0, sel.Length())
	sel = doc.Find(notifier.ContentSelector)
	sel.Each(func(_ int, s *goquery.Selection) {
		if s.Text() != "" && s.Text() != "\u00a0" {
			str := strings.ReplaceAll(s.Text(), "\u00a0", " ")
			str, _, _ = transform.String(korean.EUCKR.NewDecoder(), str)
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
		if strings.Contains(image, "base64,") {
			return
		}
		if strings.Contains(image, "fonts.gstatic.com") {
			return
		}
		if !strings.Contains(image, "http://") && !strings.Contains(image, "https://") {
			image = "http://software.ajou.ac.kr" + image
		}
		images = append(images, image)
	})

	notice := Notice{
		ID:           id,
		Title:        title,
		Department:   department,
		Date:         date,
		Url:          url,
		Content:      content,
		Images:       images,
		EnglishTopic: notifier.EnglishTopic,
		KoreanTopic:  notifier.KoreanTopic,
	}

	noticeChan <- notice
}

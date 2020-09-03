package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-redis/redis/v8"
	"hash/crc32"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

const (
	REGEX_URL         = "^[a-zA-Z0-9_:./]{5,200}$"
	REGEX_PATH        = "^/[0-9A-Za-z]{2,15}$"
	REGEX_HTTP        = "(^http://)|(^https://)"
	MAX_RANDOM_NUMBER = 1000000

	ACTUAL_DOMAIN = "localhost:8080"
)

type URLShortener interface {
	CreateNewShortcut(url string) (string, error)
	CheckShortcut(path string) error
}

type URLRepo struct {
	Conn       *redis.Client
	tpl        *template.Template
	regexUrl   *regexp.Regexp
	regexPath  *regexp.Regexp
	regexHttp  *regexp.Regexp
	crc32Table *crc32.Table
}

type ShortcutInfoTpl struct {
	NewShortcut string
	Err         error
}

func NewUrlRepo() (*URLRepo, error) {
	repo := &URLRepo{Conn: redis.NewClient(&redis.Options{
		Addr:     "redis-server:6379",
		Password: "",
		DB:       0,
	}),
		tpl:        template.Must(template.ParseGlob("tpl/*.gohtml")),
		regexUrl:   regexp.MustCompile(REGEX_URL),
		regexPath:  regexp.MustCompile(REGEX_PATH),
		regexHttp:  regexp.MustCompile(REGEX_HTTP),
		crc32Table: crc32.MakeTable(crc32.IEEE),
	}

	_, err := repo.Conn.Ping(context.TODO()).Result()

	return repo, err
}

func (r *URLRepo) CreateNewShortcut(url string) (string, error) {
	var err error
	var urlShortcut string

	// Check if url is valid
	if r.regexUrl.MatchString(url) {
		// Randomize url
		urlRandomized := fmt.Sprintf("%s%s", url, strconv.Itoa(rand.Intn(MAX_RANDOM_NUMBER)))
		// Create hash with the url
		urlShortcut = fmt.Sprintf("/%08x", crc32.Checksum([]byte(urlRandomized), r.crc32Table))
	} else {
		err = errors.New("Invalid url input")
	}

	return urlShortcut, err
}

func main() {
	rand.Seed(time.Now().UnixNano())
	repo, err := NewUrlRepo()
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/", repo.HomepageHandler)
	http.ListenAndServe(":8080", nil)
}

func (r *URLRepo) HomepageHandler(w http.ResponseWriter, req *http.Request) {
	infoTpl := ShortcutInfoTpl{}

	if req.URL.Path == "/" {
		if req.Method == http.MethodPost {
			url := req.FormValue("url")
			shortcut, err := r.CreateNewShortcut(url)
			if err != nil {
				infoTpl.Err = err
			} else {
				// Save shortcut/url in redis
				if !r.regexHttp.MatchString(url) {
					url = "http://" + url
				}

				err = r.Conn.Set(context.TODO(), shortcut, url, 0).Err()
				if err != nil {
					// Error saving to redis
					infoTpl.Err = err
				} else {
					// send shortcut to user
					infoTpl.NewShortcut = ACTUAL_DOMAIN + shortcut
				}
			}
		}
	} else {
		// Avoid URL injections
		if r.regexPath.MatchString(req.URL.Path) {
			result, err := r.Conn.Get(context.TODO(), req.URL.Path).Result()
			if err != nil {
				// shortcut not found in redis
				infoTpl.Err = errors.New("Shortcut not found")
			} else {
				http.Redirect(w, req, result, http.StatusMovedPermanently)
				return
			}
		} else {
			// notify user url is wrong formatted
			infoTpl.Err = errors.New("Url bad formatted")
		}
	}

	err := r.tpl.ExecuteTemplate(w, "homepage.gohtml", infoTpl)
	if err != nil {
		log.Fatal(err)
	}
}

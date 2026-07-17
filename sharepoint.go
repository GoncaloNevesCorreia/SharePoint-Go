package sharepoint

import (
	"fmt"
	"log"
	"reflect"
	"strings"
	"time"

	ondemand "github.com/GoncaloNevesCorreia/SharePoint-Go/auth/ondemand"
	"github.com/koltyakov/gosip"
	"github.com/koltyakov/gosip/api"
)

type Session struct {
	Client *gosip.SPClient
	SP     *api.SP
	User   *UserMetadata
}

func Init(configFile []byte) (*Session, error) {
	auth := &ondemand.AuthCnfg{}

	err := auth.ParseConfig(configFile)

	if err != nil {
		return nil, fmt.Errorf("Unable to get config: %v\n", err)
	}

	client := &gosip.SPClient{AuthCnfg: auth}

	sp := api.NewSP(client)

	return &Session{
		Client: client,
		SP:     sp,
	}, nil
}

type filter struct {
	column    string
	op        string
	value     string
	logicalOp string
}

type orderBy struct {
	column    string
	ascending bool
}

type limit struct {
	value   int
	enabled bool
}

type options struct {
	Select  []string
	Filters []*filter
	OrderBy *orderBy
	Limit   limit
}

type File struct {
	Name    string
	Content []byte
}

type UserMetadata struct {
	Id          string `json:"id"`
	DisplayName string `json:"displayName"`
	Mail        string `json:"mail"`
}

type SharePointList[T any] struct {
	user       *UserMetadata
	client     *gosip.SPClient
	api        *api.SP
	page       *api.ItemsPage
	listURI    string
	options    options
	columns    map[string]string
	payload    ItemPayload
	attachment *File
}

func NewEndpoint[T any](session *Session, listURI string) *SharePointList[T] {
	columns := getColumns[T]()

	return &SharePointList[T]{
		user:    session.User,
		client:  session.Client,
		api:     session.SP,
		listURI: listURI,
		columns: columns,
		options: options{
			Limit: limit{
				value:   10,
				enabled: true,
			},
		},
		payload: ItemPayload{},
	}

}

func getColumns[T any]() map[string]string {
	t := reflect.TypeFor[T]()

	if t.Kind() != reflect.Struct {
		panic(fmt.Errorf("%s must be a struct", t.Name()))
	}

	columns := map[string]string{}

	for field := range t.Fields() {
		if field.PkgPath != "" {
			continue
		}

		if jsonTag, ok := field.Tag.Lookup("json"); ok {

			jsonColumn, _, _ := strings.Cut(jsonTag, ",")

			if len(jsonColumn) > 0 {
				columns[jsonColumn] = field.Name
			}
		}
	}

	if len(columns) == 0 {
		panic(fmt.Errorf("No Columns found in struct '%s'. Please include the missing json tags", t.Name()))
	}

	return columns
}

func withRetryNoData(operation func() error) error {
	_, err := withRetry(func() (any, error) {
		return nil, operation()
	})

	return err
}

func withRetry[T any](operation func() (T, error)) (T, error) {
	attempts := 3
	delay := 2 * time.Second

	var err error
	var result T

	for i := 1; i <= attempts; i++ {
		result, err = operation()

		if err == nil {
			return result, nil
		}

		if strings.Contains(err.Error(), "404 Not Found") {

			return result, nil
		}

		log.Printf("Operation failed (attempt %d/%d): %v", i, attempts, err)

		if i < attempts {
			time.Sleep(delay)
		}
	}

	return result, fmt.Errorf("operation failed after %d attempts: %w", attempts, err)
}

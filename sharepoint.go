package sharepoint

import (
	"fmt"
	"reflect"
	"strings"

	ondemand "github.com/GoncaloNevesCorreia/SharePoint-Go/auth/ondemand"
	"github.com/koltyakov/gosip"
	"github.com/koltyakov/gosip/api"
)

func Init() (*api.SP, error) {
	configPath := "./config/private.json"

	auth := &ondemand.AuthCnfg{}

	err := auth.ReadConfig(configPath)

	if err != nil {
		return nil, fmt.Errorf("Unable to get config: %v\n", err)
	}

	client := &gosip.SPClient{AuthCnfg: auth}

	sp := api.NewSP(client)

	return sp, nil
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

type options struct {
	Select  []string
	Filters []*filter
	OrderBy *orderBy
	Limit   int
}

type SharePointList[T any] struct {
	api     *api.SP
	page    *api.ItemsPage
	listURI string
	options options
	columns map[string]string
	payload ItemPayload
}

func NewEndpoint[T any](sp *api.SP, listURI string) *SharePointList[T] {
	columns, err := getColumns[T]()

	if err != nil {
		panic(err)
	}

	return &SharePointList[T]{
		api:     sp,
		listURI: listURI,
		columns: columns,
		options: options{},
		payload: ItemPayload{},
	}

}

func getColumns[T any]() (map[string]string, error) {
	t := reflect.TypeFor[T]()

	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("%s must be a struct", t.Name())
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
		return nil, fmt.Errorf("No Columns found in struct '%s'. Please include the missing json tags", t.Name())
	}

	return columns, nil
}

package sharepoint

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/koltyakov/gosip/api"
)

type ItemPayload map[string]any

type SearchResponse[T any] struct {
	HasMore bool `json:"hasMore"`
	Items   []*T `json:"items"`
}

func (list *SharePointList[T]) Columns(columns []string) *SharePointList[T] {
	list.options.Select = columns

	return list
}

func (list *SharePointList[T]) Limit(value int) *SharePointList[T] {
	list.options.Limit.enabled = true
	list.options.Limit.value = value

	return list
}

func (list *SharePointList[T]) NoLimit() *SharePointList[T] {
	list.options.Limit.enabled = false

	return list
}

func (list *SharePointList[T]) Equal(column string, value string) *SharePointList[T] {
	list.setFilter(column, "eq", value)

	return list
}

func (list *SharePointList[T]) NotEqual(column string, value string) *SharePointList[T] {
	list.setFilter(column, "ne", value)

	return list
}

func (list *SharePointList[T]) GreaterThan(column string, value string) *SharePointList[T] {
	list.setFilter(column, "gt", value)

	return list
}

func (list *SharePointList[T]) GreaterThanOrEqual(column string, value string) *SharePointList[T] {
	list.setFilter(column, "ge", value)

	return list
}

func (list *SharePointList[T]) LessThan(column string, value string) *SharePointList[T] {
	list.setFilter(column, "lt", value)

	return list
}

func (list *SharePointList[T]) LessThanOrEqual(column string, value string) *SharePointList[T] {
	list.setFilter(column, "le", value)

	return list
}

func (list *SharePointList[T]) Contains(column string, value string) *SharePointList[T] {
	list.setFilter(column, "substringof", value)

	return list
}

func (list *SharePointList[T]) StartsWith(column string, value string) *SharePointList[T] {
	list.setFilter(column, "startswith", value)

	return list
}

func (list *SharePointList[T]) And() *SharePointList[T] {
	list.setLogicalOperator("and")

	return list
}

func (list *SharePointList[T]) Or() *SharePointList[T] {
	list.setLogicalOperator("or")

	return list
}

func (list *SharePointList[T]) Not() *SharePointList[T] {
	list.setLogicalOperator("not")

	return list
}

func (list *SharePointList[T]) OrderByAsc(column string) *SharePointList[T] {
	list.options.OrderBy = &orderBy{column: column, ascending: true}

	return list
}

func (list *SharePointList[T]) OrderByDesc(column string) *SharePointList[T] {
	list.options.OrderBy = &orderBy{column: column, ascending: false}

	return list
}

func (list *SharePointList[T]) GetAll() ([]*T, error) {
	items := list.getItems()

	result, err := items.GetAll()

	if err != nil {
		return nil, fmt.Errorf("Não foi possivel aceder à lista '%s': %v\n", list.listURI, err)
	}

	response := make([]*T, 0, len(result))

	for _, data := range result {

		var item *T

		if err := json.Unmarshal(data.Normalized(), &item); err != nil {
			return nil, err
		}

		response = append(response, item)
	}
	return response, nil
}

func (list *SharePointList[T]) Get() (*SearchResponse[T], error) {
	list.validateOptions()

	items := list.getItems()

	list.applySelect(items)

	list.applyFilters(items)

	list.applyOrderBy(items)

	list.applyLimit(items)

	page, err := items.GetPaged()

	list.clearFilters()

	if err != nil {
		return nil, fmt.Errorf("Não foi possivel aceder à lista '%s': %v\n", list.listURI, err)
	}

	return list.parseResponse(page)
}

func (list *SharePointList[T]) Next() (*SearchResponse[T], error) {
	if list.page == nil {
		return nil, fmt.Errorf("Não foi possivel obter a proxima pagina da lista '%s'.\n", list.listURI)
	}

	nextPage, err := list.page.GetNextPage()

	if err != nil {
		return nil, fmt.Errorf("Não foi possivel aceder à lista '%s': %v\n", list.listURI, err)
	}

	return list.parseResponse(nextPage)
}

func (list *SharePointList[T]) GetByID(itemId int) (*T, error) {

	items := list.getItems()

	data, err := items.GetByID(itemId).Get()

	if err != nil {
		if strings.HasPrefix(err.Error(), "unable to request api: 404 Not Found") {
			return nil, nil
		}

		return nil, fmt.Errorf("Não foi possivel aceder à lista '%s': %v\n", list.listURI, err)
	}

	var response *T

	if err := json.Unmarshal(data.Normalized(), &response); err != nil {
		return nil, err
	}

	return response, nil
}

func (list *SharePointList[T]) Add() (*T, error) {
	list.validatePayload()

	payload, err := json.Marshal(list.payload)

	if err != nil {
		return nil, err
	}

	items := list.getItems()

	data, err := items.Add(payload)

	if err != nil {
		return nil, fmt.Errorf("Não foi possivel adicionar à lista '%s': %v\n", list.listURI, err)
	}

	var response *T

	if err := json.Unmarshal(data.Normalized(), &response); err != nil {
		return nil, err
	}

	return response, nil
}

func (list *SharePointList[T]) Update(itemId int) error {
	list.validatePayload()

	payload, err := json.Marshal(list.payload)

	if err != nil {
		return err
	}

	items := list.getItems()

	_, err = items.GetByID(itemId).Update(payload)

	if err != nil {
		return fmt.Errorf("Não foi possivel atualizar a lista '%s': %v\n", list.listURI, err)

	}

	return nil
}

func (list *SharePointList[T]) Delete(itemId int) error {
	if itemId <= 0 {
		return fmt.Errorf("Precisa de especificar um itemID para apagar da Lista: %s\n", list.listURI)
	}

	items := list.getItems()

	err := items.GetByID(itemId).Delete()

	if err != nil {
		if strings.HasPrefix(err.Error(), "unable to request api: 404 Not Found") {
			return nil
		}

		return fmt.Errorf("Não foi possivel apagar da lista '%s': %v\n", list.listURI, err)
	}

	return nil
}

func (list *SharePointList[T]) getItems() *api.Items {
	return list.api.Web().GetList(fmt.Sprintf("Lists/%s", list.listURI)).Items()
}

func (list *SharePointList[T]) applySelect(items *api.Items) {
	if len(list.options.Select) != 0 {
		columns := list.options.Select

		items.Select(strings.Join(columns, ","))
		return
	}

	columns := slices.Collect(maps.Keys(list.columns))

	items.Select(strings.Join(columns, ","))
}

func (list *SharePointList[T]) applyFilters(items *api.Items) {
	filters := list.options.Filters

	if len(filters) == 0 {
		return
	}

	builder := strings.Builder{}

	for _, filter := range list.options.Filters {
		switch filter.op {
		case "substringof":
			fmt.Fprintf(&builder, "%s('%s',%s)", filter.op, filter.value, filter.column)

		case "startswith":
			fmt.Fprintf(&builder, "%s(%s,'%s')", filter.op, filter.column, filter.value)

		case "eq", "ne":
			fmt.Fprintf(&builder, "%s %s '%s'", filter.column, filter.op, filter.value)

		default:
			fmt.Fprintf(&builder, "%s %s %s", filter.column, filter.op, filter.value)
		}

		if filter.logicalOp != "" {
			fmt.Fprintf(&builder, " %s ", filter.logicalOp)
		}
	}

	items.Filter(builder.String())

}

func (list *SharePointList[T]) applyOrderBy(items *api.Items) {
	if list.options.OrderBy == nil {
		return
	}

	items.OrderBy(list.options.OrderBy.column, list.options.OrderBy.ascending)
}

func (list *SharePointList[T]) applyLimit(items *api.Items) {
	if !list.options.Limit.enabled {
		return
	}

	if list.options.Limit.value <= 0 {
		items.Top(10)
		return
	}

	items.Top(list.options.Limit.value)
}

func (list *SharePointList[T]) setFilter(column string, op string, value string) {
	list.options.Filters = append(list.options.Filters, &filter{column: column, op: op, value: value})
}

func (list *SharePointList[T]) setLogicalOperator(logicalOp string) {
	filters := list.options.Filters

	if len(filters) == 0 {
		return
	}

	lastFilter := filters[len(filters)-1]

	lastFilter.logicalOp = logicalOp
}

func (list *SharePointList[T]) Payload(item *T, columns ...string) {
	t := reflect.TypeFor[T]()

	list.ClearPayload()

	for _, column := range columns {
		key, ok := list.columns[column]

		if !ok {
			panic(fmt.Errorf("Column '%s' not found in struct '%s'", column, t.Name()))
		}

		reflectValue := reflect.ValueOf(item)

		value := reflectValue.Elem().FieldByName(key)

		if isTime(value.Type()) {
			list.payload[column] = value.Interface().(time.Time).Format(time.RFC3339)
		} else {
			list.payload[column] = fmt.Sprintf("%v", value)
		}

	}
}

func (list *SharePointList[T]) ClearPayload() {
	list.payload = ItemPayload{}
}

func (list *SharePointList[T]) parseResponse(page *api.ItemsPage) (*SearchResponse[T], error) {
	if page.HasNextPage() {
		list.page = page
	} else {
		list.page = nil
	}

	var items []*T

	if err := json.Unmarshal(page.Items.Normalized(), &items); err != nil {
		return nil, err
	}

	return &SearchResponse[T]{
		HasMore: list.page != nil,
		Items:   items,
	}, nil

}

func (list *SharePointList[T]) clearFilters() {
	if len(list.options.Filters) == 0 {
		return
	}

	list.options.Filters = nil
}

func isTime(t reflect.Type) bool {
	tTimeDuration := reflect.TypeFor[time.Time]()

	return t == tTimeDuration
}

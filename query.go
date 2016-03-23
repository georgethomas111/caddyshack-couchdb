package adapter

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/bushwood/caddyshack"
	"github.com/bushwood/caddyshack/resource"
	"github.com/bushwood/couchdb"
)

// View Object is placed over here as a query in couch is possible only with a view
// Helps in forming the javascript that can be used for working on stuff.
type ViewObj struct {
	Name     string
	ViewType reflect.Type
}

// FIXME : Assert Kind of the viewObj passed is a pointer.
func NewViewObj(name string, viewObj caddyshack.StoreObject) *ViewObj {
	// Check if View Exists in the DB.
	// Create View Using the tags if thats the case.
	return &ViewObj{
		Name:     name,
		ViewType: reflect.ValueOf(viewObj).Elem().Type(),
	}
}

// Create the View if it does not exist.
func (v *ViewObj) GetCondition() string {
	// Use the tags to form the condition.
	return "Not implemented"
}

// Initial Version
type CouchQuery struct {
	Condition string // Code for the view RawJson
	ViewName  string
	Store     *CouchStore
	desDoc    *couchdb.DesignDoc
}

func NewQuery(line string, viewName string, desDoc string, db *CouchStore) (couchQuery *CouchQuery) {

	// Assuming a design doc is already created.
	desDocObj := db.GetDesignDoc(desDoc)

	couchQuery = &CouchQuery{
		desDoc:    desDocObj,
		Condition: line,
		ViewName:  viewName,
		Store:     db,
	}

	// Correct the code over here.
	newView := &couchdb.View{Name: viewName}
	newView.RawStatus = true
	newView.RawJson = couchQuery.Condition

	index, status := couchQuery.desDoc.CheckExists(viewName)

	fmt.Println("Views -->", couchQuery.desDoc.Views)
	fmt.Println("LastView -->", couchQuery.desDoc.LastView)
	if status == false {
		couchQuery.desDoc.AddView(newView)

		err := couchQuery.desDoc.SaveDoc()
		if err != nil {
			panic(err)
		}

	} else {
		fmt.Println("Index found at ", index)
		if index < 0 {
			couchQuery.desDoc.LastView = newView
		} else {
			couchQuery.desDoc.Views[index] = newView
		}
	}
	return
}

// Use reflection to create the query from the tags.
func NewObjQuery(obj caddyshack.StoreObject, db *CouchStore, res *resource.Definition) (q *CouchQuery) {

	q = new(CouchQuery)
	prefix := "doc"

	viewName := q.getViewName(obj)
	view := couchdb.NewView(viewName, prefix, q.getCondition(obj, prefix), q.getEmits(obj))
	//Creates the DesignDoc if it does not exist.
	desDoc := db.GetDesignDoc(res.DesDoc)
	index, status := desDoc.CheckExists(viewName)

	fmt.Println("Index, Status", index, status)

	if status == false {
		desDoc.AddView(view)
		fmt.Println("Added view", desDoc, view)
	} else {
		if index < 0 {
			desDoc.LastView = view
		} else {
			desDoc.Views[index] = view
		}
	}

	err := desDoc.SaveDoc()
	if err != nil {
		panic(err)
	}

	q.desDoc = desDoc
	q.ViewName = viewName
	q.Store = db

	return
}

func (q *CouchQuery) SetCondition(cond string) {
	q.Condition = cond
}

func (q *CouchQuery) GetCondition() string {
	return q.Condition
}

func (q *CouchQuery) getViewName(obj caddyshack.StoreObject) string {
	structObj := reflect.ValueOf(obj).Elem()
	typeOfObj := structObj.Type()
	return strings.ToLower(typeOfObj.Name())
}

func (q *CouchQuery) getCondition(obj caddyshack.StoreObject, prefix string) (condStr string) {

	structObj := reflect.ValueOf(obj).Elem()
	typeOfObj := structObj.Type()

	firstCond := true

	if structObj.Kind() == reflect.Struct {
		for index := 0; index < typeOfObj.NumField(); index++ {
			structField := typeOfObj.Field(index)
			fieldCond := structField.Tag.Get("condition")
			if fieldCond != "" {
				if firstCond {
					condStr = condStr + prefix + "." + fieldCond
					firstCond = false
				} else {
					condStr = condStr + " && " + prefix + "." + fieldCond
				}
			}
		}
	} else {
		panic(errors.New("Expected a struct pointer as input."))
	}

	return
}

func (q *CouchQuery) getEmits(obj caddyshack.StoreObject) (emits string) {

	structObj := reflect.ValueOf(obj).Elem()
	typeOfObj := structObj.Type()

	firstCond := true

	if structObj.Kind() == reflect.Struct {
		for index := 0; index < typeOfObj.NumField(); index++ {
			structField := typeOfObj.Field(index)
			jsonStr := structField.Tag.Get("json")
			if jsonStr != "" {
				if firstCond {
					emits = emits + "\\\"" + jsonStr + "\\\""
					firstCond = false
				} else {
					emits = emits + ", \\\"" + jsonStr + "\\\""
				}
			}
		}
	} else {
		panic(errors.New("Expected a struct pointer as input."))
	}

	return
}

func (q *CouchQuery) MarshalStoreObjects(data []byte) (err error, result []caddyshack.StoreObject) {

	jsonStream := strings.NewReader(string(data))
	jsonDecoder := json.NewDecoder(jsonStream)

	type ObjInfo struct {
		NumRows int               `json:"total_rows"`
		Offset  int               `json:"offset"`
		Array   []json.RawMessage `json:"rows"`
	}

	objInfo := new(ObjInfo)

	err = jsonDecoder.Decode(objInfo)

	for _, row := range objInfo.Array {
		// Does the reflection part
		err, storeObj := q.Store.GetStoreObj(row)
		if err != nil {
			err = errors.New("Marshal Object" + err.Error())
		}
		result = append(result, storeObj)
	}

	return
}

func (q *CouchQuery) Execute() (error, []caddyshack.StoreObject) {
	// Currently O(n) w.r.t to views

	err, data := q.desDoc.Db.GetView(q.desDoc.Id, q.ViewName)
	if err != nil {
		return errors.New("Error retreiving view : " + err.Error()), nil
	} else {
		// Print for now create store Object later.
		// FIXME Handle unmarshalling over here.
		err, result := q.MarshalStoreObjects(data)
		if err != nil {
			return errors.New("Could not Marshal json" + err.Error()), result
		}
		return nil, result
	}

	// Move this section to the New
	// The intutive version would be creating an object and then adding methods to it.

	// If it exists get the view back.
	// Otherwise Get Retrieve the Data and Marshal the store Object from the json..
}
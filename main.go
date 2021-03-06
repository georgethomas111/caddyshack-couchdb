package adapter

import (
	"encoding/json"

	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/gospackler/caddyshack"
	"github.com/gospackler/caddyshack/model"
	"github.com/gospackler/caddyshack/resource"
	"github.com/gospackler/couchdb"
)

type CouchStore struct {
	model  *model.Definition
	client *couchdb.Client
	DbObj  *couchdb.Database
	DesDoc map[string]*couchdb.DesignDoc

	Res *resource.Definition

	// Fields for caddyshack follows
	// This is needed for identifying adpter for caddyshack.
	StoreName string
	// For all queries, receiver for data.
	ObjType  reflect.Type
	DefQuery *CouchQuery
}

// FIXME : Assert Kind of the objModel passed is a pointer.
func NewCouchStore(res *resource.Definition, objModel caddyshack.StoreObject) (c *CouchStore) {

	objType := reflect.ValueOf(objModel).Elem().Type()
	client := couchdb.NewClient(res.Host, res.Port)
	c = &CouchStore{
		client:    &client,
		StoreName: "couchdb",
		ObjType:   objType,
		DesDoc:    make(map[string]*couchdb.DesignDoc),
		Res:       res,
	}

	dbObj := c.client.DB(res.Name)
	status, err := dbObj.Exists()
	if err == nil {
		if status == false {
			err = dbObj.Create()
			if err != nil {
				panic("Could not create a database " + err.Error())
			}
		}
	} else {
		panic("Could not connect with db " + err.Error())
	}
	c.DbObj = &dbObj
	c.DefQuery = NewObjQuery(objModel, c)
	return
}

// FIXME Reason out and remove this method in future.
func (c *CouchStore) Init(model *model.Definition) (error, caddyshack.Store) {
	c.model = model
	return nil, c
}

func (c *CouchStore) GetDesignDoc(docName string) *couchdb.DesignDoc {

	_, exists := c.DesDoc[docName] //Checking if the view exists.

	// FIXME check in the db as well to make sure the document does not exist there.

	if exists == true {

		return c.DesDoc[docName]
	} else {

		err, doc := couchdb.RetreiveDocFromDb(docName, c.DbObj)
		log.Info("Checking if document with name ", docName, " is present.")
		if err != nil {
			c.DesDoc[docName] = couchdb.NewDesignDoc(docName, c.DbObj)
		} else {
			c.DesDoc[docName] = doc
		}
		return c.DesDoc[docName]
	}
}

func (c *CouchStore) GetName() string {

	return c.StoreName
}

func (c *CouchStore) SetName(name string) error {

	c.StoreName = name
	return nil
}

// TODO : This method could be part of the interface in general which can be overridden
// Does it work that way ??
func (c *CouchStore) verify(obj caddyshack.StoreObject) {

}

func (c *CouchStore) Create(obj caddyshack.StoreObject) (err error) {

	strObj, err := json.Marshal(obj)

	doc := couchdb.NewDocument("", "", c.DbObj)
	err = doc.Create(strObj)
	obj.SetKey(doc.Id)
	return
}

// {"id":"bb0c3212953bdc7fad2ade66160c244d","key":"bb0c3212953bdc7fad2ade66160c244d","value":{"name":"abcd","surprise":"1234","field1":"field1","age":0,"id":""}}
// An example of json that could come up.
// Decodes a key : value type to a the registered object and returns it.
func (c *CouchStore) GetStoreObj(jsonObj []byte) (error, caddyshack.StoreObject) {

	jsonStream := bytes.NewBuffer(jsonObj)
	jsonDecoder := json.NewDecoder(jsonStream)

	type Object struct {
		Id    string          `json:"id"`
		Key   interface{}     `json:"key"`
		Value json.RawMessage `json:"value"`
	}

	tempObj := new(Object)
	err := jsonDecoder.Decode(tempObj)
	if err != nil {
		return err, nil
	}

	dynmaicObj := reflect.New(c.ObjType).Interface()
	err = json.Unmarshal(tempObj.Value, dynmaicObj)
	if err != nil {
		return err, nil
	}

	obj := dynmaicObj.(caddyshack.StoreObject)
	return nil, obj
}

func (c *CouchStore) MarshalStoreObjects(data []byte) (result []caddyshack.StoreObject, err error) {

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
		err, storeObj := c.GetStoreObj(row)
		if err != nil {
			err = errors.New("Marshal Object" + err.Error())
			break
		}
		result = append(result, storeObj)
	}

	return
}

func (c *CouchStore) ReadOne(key string) (error, caddyshack.StoreObject) {

	log.Info("ReadOne : Key = ", key)
	doc := couchdb.NewDocument(key, "", c.DbObj)
	jsonObj, err := doc.GetDocument()
	if err != nil {
		return err, nil
	}
	log.Debug("Read one resp :", string(jsonObj))

	//	err, obj := c.GetStoreObj(jsonObj)
	dynmaicObj := reflect.New(c.ObjType).Interface()
	err = json.Unmarshal(jsonObj, dynmaicObj)
	if err != nil {
		return err, nil
	}

	obj := dynmaicObj.(caddyshack.StoreObject)
	obj.SetKey(doc.Id)
	return err, obj
}

func (c *CouchStore) ReadOneFromView(desDocName string, viewName string, key string) (caddyshack.StoreObject, error) {

	if !strings.Contains(desDocName, "/") {
		desDocName = "_design/" + desDocName
	}
	log.Debug("Trying to read key " + key + " in viewName " + viewName + " of desDoc " + desDocName)
	data, err := c.DbObj.GetView(desDocName, viewName, key)

	if err != nil {
		newErr := fmt.Errorf("Error retreiving : Key = %s ViewName = %s desDoc = %s :  %s", key, viewName, desDocName, err.Error())
		return nil, newErr
	} else {
		// Print for now create store Object later.
		// FIXME Handle unmarshalling over here.
		result, err := c.MarshalStoreObjects(data)
		if err != nil {
			return nil, errors.New("Could not Marshal json" + err.Error())
		}
		if len(result) < 1 {
			return nil, errors.New("caddyshack-couchdb : Key not found in database ")
		}
		return result[0], nil
	}
}

func (c *CouchStore) Read(query caddyshack.Query) (error, []caddyshack.StoreObject) {

	err, objects := query.Execute()
	// Use the rawJson to check for the view.

	return err, objects
}

// Read Default uses the default query object to make the request.
func (c *CouchStore) ReadDef() (err error, objects []caddyshack.StoreObject) {

	err, objects = c.Read(c.DefQuery)
	if err != nil {
		err = errors.New("Read Def :" + err.Error())
	}
	return
}

// The object passed should have CouchWrapperUpdate as an anonymous field containing the details.
func (c *CouchStore) UpdateOne(obj caddyshack.StoreObject) (err error) {

	// FIXME Actually a hack which works because of the implementation.
	byteObj, err := json.Marshal(obj)
	doc := couchdb.NewDocument(obj.GetKey(), "", c.DbObj)
	err = doc.Update(byteObj)
	return
}

func (c *CouchStore) DestroyOne(key string) error {
	// Destroy not yet implemented need to implement it in the lower level. Missed it!
	return nil
}

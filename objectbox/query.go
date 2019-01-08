/*
 * Copyright 2018 ObjectBox Ltd. All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package objectbox

/*
#cgo LDFLAGS: -lobjectbox
#include <stdlib.h>
#include "objectbox.h"
*/
import "C"
import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"unsafe"
)

// A Query allows to search for objects matching user defined conditions.
//
// For example, you can find all people whose last name starts with an 'N':
// 		box.Query(Person_.LastName.HasPrefix("N", false)).Find()
// Note that Person_ is a struct generated by ObjectBox allowing to conveniently reference properties.
type Query struct {
	typeId     TypeId
	objectBox  *ObjectBox
	cQuery     *C.OBX_query
	closeMutex sync.Mutex
	offset     uint64
	limit      uint64
}

// Frees (native) resources held by this Query.
// Note that this is optional and not required because the GC invokes a finalizer automatically.
func (query *Query) Close() error {
	query.closeMutex.Lock()
	defer query.closeMutex.Unlock()
	if query.cQuery != nil {
		rc := C.obx_query_close(query.cQuery)
		query.cQuery = nil
		if rc != 0 {
			return createError()
		}
	}
	return nil
}

func queryFinalizer(query *Query) {
	err := query.Close()
	if err != nil {
		fmt.Printf("Error while finalizer closed query: %s", err)
	}
}

// The native query object in the ObjectBox core is not tied with other resources.
// Thus timing of the Close call is independent from other resources.
func (query *Query) installFinalizer() {
	runtime.SetFinalizer(query, queryFinalizer)
}

func (query *Query) errorClosed() error {
	return errors.New("illegal state; query was closed")
}

// Find returns all objects matching the query
func (query *Query) Find() (objects interface{}, err error) {
	err = query.objectBox.runWithCursor(query.typeId, true, func(cursor *cursor) error {
		var errInner error
		objects, errInner = query.find(cursor)
		return errInner
	})
	return
}

// Offset defines the index of the first object to process (how many objects to skip)
func (query *Query) Offset(offset uint64) *Query {
	query.offset = offset
	return query
}

// Limit sets the number of elements to process by the query
func (query *Query) Limit(limit uint64) *Query {
	query.limit = limit
	return query
}

// FindIds returns IDs of all objects matching the query
func (query *Query) FindIds() (ids []uint64, err error) {
	err = query.objectBox.runWithCursor(query.typeId, true, func(cursor *cursor) error {
		var errInner error
		ids, errInner = query.findIds(cursor)
		return errInner
	})

	// TODO pass offset & limit to the underlying C call for more efficiency (not supported yet by the C-API)
	if query.offset != 0 || query.limit != 0 && err == nil && ids != nil {
		var endOffset = uint64(len(ids))
		if query.limit != 0 && query.offset+query.limit < endOffset {
			endOffset = query.offset + query.limit
		}
		return ids[query.offset:endOffset], nil
	}

	return
}

// Count returns the number of objects matching the query
func (query *Query) Count() (count uint64, err error) {
	// doesn't support offset/limit at this point
	if query.offset != 0 || query.limit != 0 {
		return 0, fmt.Errorf("limit/offset are not supported by Count at this moment")
	}

	err = query.objectBox.runWithCursor(query.typeId, true, func(cursor *cursor) error {
		var errInner error
		count, errInner = query.count(cursor)
		return errInner
	})

	return
}

// Remove permanently deletes all objects matching the query from the database
func (query *Query) Remove() (count uint64, err error) {
	// doesn't support offset/limit at this point
	if query.offset != 0 || query.limit != 0 {
		return 0, fmt.Errorf("limit/offset are not supported by Remove at this moment")
	}

	err = query.objectBox.runWithCursor(query.typeId, false, func(cursor *cursor) error {
		var errInner error
		count, errInner = query.remove(cursor)
		return errInner
	})

	return
}

// Describe returns a string representation of the query
func (query *Query) Describe() (string, error) {
	if query.cQuery == nil {
		return "", query.errorClosed()
	}
	// no need to free, it's handled by the cQuery internally
	cResult := C.obx_query_describe_params(query.cQuery)

	return C.GoString(cResult), nil
}

func (query *Query) count(cursor *cursor) (uint64, error) {
	if query.cQuery == nil {
		return 0, query.errorClosed()
	}
	var cCount C.uint64_t
	rc := C.obx_query_count(query.cQuery, cursor.cursor, &cCount)
	if rc != 0 {
		return 0, createError()
	}
	return uint64(cCount), nil
}

func (query *Query) remove(cursor *cursor) (uint64, error) {
	if query.cQuery == nil {
		return 0, query.errorClosed()
	}
	var cCount C.uint64_t
	rc := C.obx_query_remove(query.cQuery, cursor.cursor, &cCount)
	if rc != 0 {
		return 0, createError()
	}
	return uint64(cCount), nil
}

func (query *Query) findIds(cursor *cursor) (ids []uint64, err error) {
	if query.cQuery == nil {
		return nil, query.errorClosed()
	}
	cIdsArray := C.obx_query_find_ids(query.cQuery, cursor.cursor)
	if cIdsArray == nil {
		return nil, createError()
	}

	idsArray := cIdsArrayToGo(cIdsArray)
	defer idsArray.free()

	return idsArray.ids, nil
}

func (query *Query) find(cursor *cursor) (slice interface{}, err error) {
	if query.cQuery == nil {
		return 0, query.errorClosed()
	}
	cBytesArray := C.obx_query_find(query.cQuery, cursor.cursor, C.uint64_t(query.offset), C.uint64_t(query.limit))
	if cBytesArray == nil {
		return nil, createError()
	}

	return cursor.cBytesArrayToObjects(cBytesArray), nil
}

type Property interface {
	propertyId() TypeId
	entityId() TypeId
}

func (query *Query) SetStringParams(property Property, values ...string) error {
	var rc = 0
	if len(values) == 0 {
		return fmt.Errorf("no values given")

	} else if len(values) == 1 {
		cString := C.CString(values[0])
		defer C.free(unsafe.Pointer(cString))
		rc = int(C.obx_query_string_param(query.cQuery, C.obx_schema_id(property.entityId()), C.obx_schema_id(property.propertyId()), cString))

	} else {
		return fmt.Errorf("too many values given")
	}

	if rc != 0 {
		return createError()
	}
	return nil
}

func (query *Query) SetStringParamsIn(property Property, values ...string) error {
	var rc = 0
	if len(values) == 0 {
		return fmt.Errorf("no values given")

	} else {
		cStringArray := goStringArrayToC(values)
		defer cStringArray.free()
		rc = int(C.obx_query_string_params_in(query.cQuery, C.obx_schema_id(property.entityId()), C.obx_schema_id(property.propertyId()), cStringArray.cArray, C.int(cStringArray.size)))
	}

	if rc != 0 {
		return createError()
	}
	return nil
}

func (query *Query) SetInt64Params(property Property, values ...int64) error {
	var rc = 0
	if len(values) == 0 {
		return fmt.Errorf("no values given")

	} else if len(values) == 1 {
		rc = int(C.obx_query_int_param(query.cQuery, C.obx_schema_id(property.entityId()), C.obx_schema_id(property.propertyId()), C.int64_t(values[0])))

	} else if len(values) == 2 {
		rc = int(C.obx_query_int_params(query.cQuery, C.obx_schema_id(property.entityId()), C.obx_schema_id(property.propertyId()), C.int64_t(values[0]), C.int64_t(values[1])))

	} else {
		return fmt.Errorf("too many values given")
	}

	if rc != 0 {
		return createError()
	}
	return nil
}

func (query *Query) SetInt64ParamsIn(property Property, values ...int64) error {
	var rc = 0
	if len(values) == 0 {
		return fmt.Errorf("no values given")

	} else {
		rc = int(C.obx_query_int64_params_in(query.cQuery, C.obx_schema_id(property.entityId()), C.obx_schema_id(property.propertyId()), (*C.int64_t)(unsafe.Pointer(&values[0])), C.int(len(values))))
	}

	if rc != 0 {
		return createError()
	}
	return nil
}

func (query *Query) SetInt32ParamsIn(property Property, values ...int32) error {
	var rc = 0
	if len(values) == 0 {
		return fmt.Errorf("no values given")

	} else {
		rc = int(C.obx_query_int32_params_in(query.cQuery, C.obx_schema_id(property.entityId()), C.obx_schema_id(property.propertyId()), (*C.int32_t)(unsafe.Pointer(&values[0])), C.int(len(values))))
	}

	if rc != 0 {
		return createError()
	}
	return nil
}

func (query *Query) SetFloat64Params(property Property, values ...float64) error {
	var rc = 0
	if len(values) == 0 {
		return fmt.Errorf("no values given")

	} else if len(values) == 1 {
		rc = int(C.obx_query_double_param(query.cQuery, C.obx_schema_id(property.entityId()), C.obx_schema_id(property.propertyId()), C.double(values[0])))

	} else if len(values) == 2 {
		rc = int(C.obx_query_double_params(query.cQuery, C.obx_schema_id(property.entityId()), C.obx_schema_id(property.propertyId()), C.double(values[0]), C.double(values[1])))

	} else {
		return fmt.Errorf("too many values given")
	}

	if rc != 0 {
		return createError()
	}
	return nil
}

func (query *Query) SetBytesParams(property Property, values ...[]byte) error {
	var rc = 0
	if len(values) == 0 {
		return fmt.Errorf("no values given")

	} else if len(values) == 1 {
		rc = int(C.obx_query_bytes_param(query.cQuery, C.obx_schema_id(property.entityId()), C.obx_schema_id(property.propertyId()), cBytesPtr(values[0]), C.size_t(len(values[0]))))

	} else {
		return fmt.Errorf("too many values given")
	}

	if rc != 0 {
		return createError()
	}
	return nil
}

package templates

import (
	"strings"
	"text/template"
)

var funcMap = template.FuncMap{
	"StringTitle": strings.Title,
	"StringCamel": func(s string) string {
		result := strings.Title(s)
		return strings.ToLower(result[0:1]) + result[1:]
	},
}

var BindingTemplate = template.Must(template.New("binding").Funcs(funcMap).Parse(
	`// This file was automatically generated by ObjectBox, do not modify

package {{.Package}}

import (
	"github.com/google/flatbuffers/go"
	"github.com/objectbox/objectbox-go/objectbox"
	"github.com/objectbox/objectbox-go/objectbox/fbutils"
)

{{range $entity := .Entities -}}
{{$entityNameCamel := $entity.Name | StringCamel -}}
type {{$entityNameCamel}}EntityInfo struct {
	Id objectbox.TypeId
	Uid uint64
}

var {{$entity.Name}}Binding = {{$entityNameCamel}}EntityInfo {
	Id: {{$entity.Id}}, 
	Uid: {{$entity.Uid}},
}

func ({{$entityNameCamel}}EntityInfo) AddToModel(model *objectbox.Model) {
    model.Entity("{{$entity.Name}}", {{$entity.Id}}, {{$entity.Uid}})
    {{range $property := $entity.Properties -}}
    model.Property("{{$property.ObName}}", objectbox.PropertyType_{{$property.ObType}}, {{$property.Id}}, {{$property.Uid}})
    {{if len $property.ObFlags -}}
        model.PropertyFlags(
        {{- range $key, $flag := $property.ObFlags -}}
            {{if gt $key 0}} | {{end}}objectbox.PropertyFlags_{{$flag}}
        {{- end}})
        {{- /* TODO model.PropertyIndexId() && model.PropertyRelation() */}}
    {{end -}}
    {{end -}}
    model.EntityLastPropertyId({{$entity.LastPropertyId.GetId}}, {{$entity.LastPropertyId.GetUid}})
}

func ({{$entityNameCamel}}EntityInfo) GetId(entity interface{}) (uint64, error) {
	return entity.(*{{$entity.Name}}).{{$entity.IdProperty.Name}}, nil
}

func ({{$entityNameCamel}}EntityInfo) Flatten(entity interface{}, fbb *flatbuffers.Builder, id uint64) {
    {{if $entity.HasNonIdProperty}}ent := entity.(*{{$entity.Name}}){{end -}}

    {{- range $property := $entity.Properties}}
        {{- if eq $property.FbType "UOffsetT"}}
            {{- if eq $property.GoType "string"}}
    var offset{{$property.Name}} = fbutils.CreateStringOffset(fbb, ent.{{$property.Name}})
            {{- else if eq $property.GoType "[]byte"}}
    var offset{{$property.Name}} = fbutils.CreateByteVectorOffset(fbb, ent.{{$property.Name}})
            {{- else -}}
            TODO offset creation for the {{$property.Name}}, type ${{$property.GoType}} is not implemented
            {{- end -}}
        {{end}}
    {{- end}}

    // build the FlatBuffers object
    fbb.StartObject({{$entity.LastPropertyId.GetId}})
    {{range $property := $entity.Properties -}}
    fbb.Prepend{{$property.FbType}}Slot({{$property.FbSlot}},
        {{- if eq $property.FbType "UOffsetT"}} offset{{$property.Name}}, 0)
        {{- else if eq $property.Name $entity.IdProperty.Name}} id, 0)
        {{- else if eq $property.GoType "bool"}} ent.{{$property.Name}}, false)
        {{- else if eq $property.GoType "int"}} int32(ent.{{$property.Name}}), 0)
        {{- else if eq $property.GoType "uint"}} uint32(ent.{{$property.Name}}), 0)
        {{- else}} ent.{{$property.Name}}, 0)
        {{- end}}
    {{end -}}
}

func ({{$entityNameCamel}}EntityInfo) ToObject(bytes []byte) interface{} {
	table := fbutils.GetRootAsTable(bytes, flatbuffers.UOffsetT(0))

	return &{{$entity.Name}}{
	{{- range $property := $entity.Properties}}
		{{$property.Name}}: {{if eq $property.GoType "bool"}} table.GetBoolSlot({{$property.FbvTableOffset}}, false)
        {{- else if eq $property.GoType "int"}} int(table.GetUint32Slot({{$property.FbvTableOffset}}, 0))
        {{- else if eq $property.GoType "uint"}} uint(table.GetUint32Slot({{$property.FbvTableOffset}}, 0))
		{{- else if eq $property.GoType "rune"}} rune(table.GetInt32Slot({{$property.FbvTableOffset}}, 0))
		{{- else if eq $property.GoType "string"}} table.GetStringSlot({{$property.FbvTableOffset}})
        {{- else if eq $property.GoType "[]byte"}} table.GetByteVectorSlot({{$property.FbvTableOffset}})
		{{- else}} table.Get{{$property.GoType | StringTitle}}Slot({{$property.FbvTableOffset}}, 0)
        {{- end}},
	{{- end}}
	}
}

func ({{$entityNameCamel}}EntityInfo) MakeSlice(capacity int) interface{} {
	return make([]*{{$entity.Name}}, 0, capacity)
}

func ({{$entityNameCamel}}EntityInfo) AppendToSlice(slice interface{}, entity interface{}) interface{} {
	return append(slice.([]*{{$entity.Name}}), entity.(*{{$entity.Name}}))
}

type {{$entity.Name}}Box struct {
	*objectbox.Box
}

func BoxFor{{$entity.Name}}(ob *objectbox.ObjectBox) *{{$entity.Name}}Box {
	return &{{$entity.Name}}Box{
		Box: ob.Box({{$entity.Id}}),
	}
}

func (box *{{$entity.Name}}Box) Get(id uint64) (*{{$entity.Name}}, error) {
	entity, err := box.Box.Get(id)
	if err != nil {
		return nil, err
	} else if entity == nil {
		return nil, nil
	}
	return entity.(*{{$entity.Name}}), nil
}

func (box *{{$entity.Name}}Box) GetAll() ([]*{{$entity.Name}}, error) {
	entities, err := box.Box.GetAll()
	if err != nil {
		return nil, err
	}
	return entities.([]*{{$entity.Name}}), nil
}

func (box *{{$entity.Name}}Box) Remove(entity *{{$entity.Name}}) (err error) {
	return box.Box.Remove(entity.{{$entity.IdProperty.Name}})
}

{{end -}}`))

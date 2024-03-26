package modelchecker

import (
	ast "fizz/proto"
	"fmt"
	"github.com/golang/protobuf/proto"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

var re = regexp.MustCompile(`Stmts\[\d+\]`)

type ProtoPath struct {
	// TODO(jayaprabhakar): A quick hack, fix this. It is safe because this field is immutable.
	filesMap map[*ast.File]map[string]proto.Message
}
var protoPathInstance = &ProtoPath{filesMap: make(map[*ast.File]map[string]proto.Message)}

func GetProtoFieldByPath(file *ast.File, location string) proto.Message {
	if protoPathInstance.filesMap[file] == nil {
		protoPathInstance.filesMap[file] = make(map[string]proto.Message)
	} else if val, ok := protoPathInstance.filesMap[file][location]; ok {
		return val
	}
	field := GetFieldByPath(file, location)
	if field == nil {
		protoPathInstance.filesMap[file][location] = nil
		return nil
	}
	protobuf := convertToProto(field.Elem().Interface(), field.Type())
	protoPathInstance.filesMap[file][location] = protobuf
	return protobuf
}

func GetStringFieldByPath(file *ast.File, location string) (string, bool) {
	field := GetFieldByPath(file, location)
	if field == nil {
		return "", false
	}
	t := field.Type()
	if t.Kind() == reflect.String {
		str := field.Interface().(string)
		return str, true
	}
	return "", false
}

func convertToProto(value interface{}, messageType reflect.Type) proto.Message {
	// Create a new instance of the protobuf message type
	protoInstance := reflect.New(messageType.Elem()).Interface().(proto.Message)

	// Use reflection to set the value of the message
	protoValue := reflect.ValueOf(protoInstance).Elem()
	protoValue.Set(reflect.ValueOf(value))

	return protoInstance.(proto.Message)
}

func GetFieldByPath(msg proto.Message, path string) *reflect.Value {
	v := reflect.ValueOf(msg).Elem()
	parts := strings.Split(path, ".")
	//fmt.Printf("before loop, %+v\n", v)
	//fmt.Printf("parts, %+v\n", parts)

	for _, part := range parts {
		if strings.Contains(part, "[") && strings.Contains(part, "]") {
			// Handle repeated field index
			indexedFieldParts := strings.Split(part, "[")
			fieldName := indexedFieldParts[0]
			indexStr := strings.Split(indexedFieldParts[1], "]")[0]
			index, err := strconv.Atoi(indexStr)
			if err != nil {
				panic(err)
				return nil
			}
			if v.Kind() == reflect.Ptr {
				v = v.Elem()
			}
			field := v.FieldByName(fieldName)
			//fmt.Printf("index: %d, fieldName: %s, field: %+v\n", index, fieldName, field)
			if !field.IsValid() || field.Kind() != reflect.Slice {
				return nil
			}
			if index < 0 || index >= field.Len() {
				return nil
			}

			v = field.Index(index)
		} else {
			// Handle regular fields
			field := v.Elem().FieldByName(part)
			if !field.IsValid() {
				return nil
			}
			v = field
		}
	}

	return &v //.Interface()
}

func GetNextFieldPath(msg proto.Message, path string) (string, *reflect.Value) {
	v := reflect.ValueOf(msg).Elem()
	_ = v
	parts := strings.Split(path, ".")
	for i := len(parts) - 1; i >= 0; i-- {
		part := parts[i]
		if strings.Contains(part, "[") && strings.Contains(part, "]") {
			fieldName := strings.Split(part, "[")[0]
			indexStr := strings.Split(strings.Split(part, "[")[1], "]")[0]
			index, err := strconv.Atoi(indexStr)
			if err != nil {
				panic(err)
				return "", nil
			}
			if fieldName != "Stmts" {
				continue
			}
			nextFieldName := ""
			if i > 0 {
				prefix := strings.Join(parts[0:i], ".")
				nextFieldName = fmt.Sprintf("%s.%s[%d]", prefix, fieldName, index+1)
			} else {
				nextFieldName = fmt.Sprintf("%s[%d]", fieldName, index+1)
			}

			nextField := GetFieldByPath(msg, nextFieldName)
			if nextField != nil {
				return nextFieldName, nextField
			}
			nextFieldName = ""
			if i > 0 {
				prefix := strings.Join(parts[0:i], ".")
				nextFieldName = fmt.Sprintf("%s.$", prefix)
			} else {
				nextFieldName = fmt.Sprintf("%s[%d]", fieldName, index+1)
			}
			return nextFieldName, nil
		}
	}
	return "", nil
}

func ParentBlockPath(path string) string {
	lastIndex := strings.LastIndex(path, ".Block")
	if lastIndex == -1 {
		return ""
	}
	return path[:lastIndex] + ".Block"
}

func RemoveLastBlock(path string) string {
	return RemoveLastSegment(path, ".Block")
}

func RemoveLastForStmt(path string) string {
	return RemoveLastSegment(path, ".ForStmt")
}

func RemoveLastWhileStmt(path string) string {
	return RemoveLastSegment(path, ".WhileStmt")
}

func RemoveLastLoop(path string) string {
	if strings.LastIndex(path, ".ForStmt") > strings.LastIndex(path, ".WhileStmt") {
		return RemoveLastSegment(path, ".ForStmt")
	} else {
		return RemoveLastSegment(path, ".WhileStmt")
	}
}

func RemoveLastLoopBlock(path string) string {
	if strings.LastIndex(path, ".ForStmt") > strings.LastIndex(path, ".WhileStmt") {
		newPath := RemoveLastSegment(path, ".ForStmt")
		return fmt.Sprintf("%s.ForStmt.Block.$", newPath)
	} else {
		newPath := RemoveLastSegment(path, ".WhileStmt")
		return fmt.Sprintf("%s.WhileStmt.Block.$", newPath)
	}

}

func RemoveLastSegment(path string, substr string) string {
	lastIndex := strings.LastIndex(path, substr)
	if lastIndex == -1 {
		return ""
	}
	return path[:lastIndex]
}

func EndOfBlock(path string) string {
	return replaceLastStmts(path, "$")
}

func replaceLastStmts(input, replacement string) string {

	matches := re.FindAllStringIndex(input, -1)

	if matches == nil {
		// Pattern not found
		return input
	}

	lastMatch := matches[len(matches)-1]
	result := input[:lastMatch[0]] + replacement + input[lastMatch[1]:]
	return result
}

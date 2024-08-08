package lib

import (
    "encoding/json"
    "google.golang.org/protobuf/encoding/protojson"
    "google.golang.org/protobuf/proto"
    "gopkg.in/yaml.v3"
    "io"
    "os"
)

func ReadProtoFromFile(filename string, protomsg proto.Message) error {
    yamlFile, err := os.Open(filename)
    if err != nil {
        return err
    }
    defer yamlFile.Close()
    yamlBytes, _ := io.ReadAll(yamlFile)
    return ReadProtoFromBytes(yamlBytes, protomsg)
}

func ReadProtoFromBytes(yamlBytes []byte, protomsg proto.Message) error {
    jsonBytes, err := YamlToJson(yamlBytes)
    if err != nil {
        return err
    }

    return protojson.Unmarshal(jsonBytes, protomsg)
}

// JsonToYaml converts JSON data to YAML data
func JsonToYaml(jsonData []byte) ([]byte, error) {
    var data interface{}

    err := json.Unmarshal(jsonData, &data)
    if err != nil {
        return nil, err
    }

    yamlData, err := yaml.Marshal(&data)
    if err != nil {
        return nil, err
    }

    return yamlData, nil
}

// YamlToJson converts YAML data to JSON data
func YamlToJson(yamlData []byte) ([]byte, error) {
    var data interface{}

    err := yaml.Unmarshal(yamlData, &data)
    if err != nil {
        return nil, err
    }

    jsonData, err := json.Marshal(&data)
    if err != nil {
        return nil, err
    }

    return jsonData, nil
}


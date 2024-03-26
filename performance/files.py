import glob
import json
import os
import yaml
import json
from google.protobuf import json_format

import proto.graph_pb2 as graph
import proto.fizz_ast_pb2 as ast
import proto.performance_model_pb2 as perf


def yaml_to_proto(yaml_filename, proto_msg):
    # Read the YAML file
    with open(yaml_filename, 'r') as yaml_file:
        yaml_content = yaml.safe_load(yaml_file)

    # Convert YAML content to JSON string
    json_str = json.dumps(yaml_content)

    # Parse JSON string into Proto message
    json_format.Parse(json_str, proto_msg)

    return proto_msg


def json_to_proto(json_filename, proto_msg):
    # Read the YAML file
    with open(json_filename, 'r') as json_file:
        json_content = json.load(json_file)

    json_str = json.dumps(json_content)
    # Parse JSON string into Proto message
    json_format.Parse(json_str, proto_msg)

    return proto_msg


def load_adj_lists_from_proto_files(path_prefix):
    pattern = os.path.join(f"{path_prefix}*adjacency_lists_*.pb")
    links = graph.Links()
    return load_proto_files(pattern, links)


def load_nodes_from_proto_files(path_prefix):
    pattern = os.path.join(f"{path_prefix}*nodes_*.pb")
    pb = graph.Nodes()
    return load_proto_files(pattern, pb)


def load_proto_files(pattern, pb):
    file_paths = glob.glob(pattern)
    for file_path in file_paths:
        with open(file_path, "rb") as f:
            pb.MergeFromString(f.read())
            # print(pb)
    return pb


def load_behavior_model_from_file(file_path):
    # Create an empty PerformanceModel message
    model = ast.File()

    json_to_proto(file_path, model)

    return model


def load_performance_model_from_file(file_path):
    # Create an empty PerformanceModel message
    perf_model = perf.PerformanceModel()

    yaml_to_proto(file_path, perf_model)

    return perf_model


def get_state_string(node):
    node_str = ""
    if 'state' in node:
        node_str += f"state: {node['state']} / "

    if 'returns' in node:
        node_str += f"returns: {node['returns']}"

    return node_str

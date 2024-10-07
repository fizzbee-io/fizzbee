import json
import base64
import sys
import urllib.parse

# Function to remove 'Messages' and 'Labels' fields from the object recursively
def remove_fields(obj):
    if isinstance(obj, dict):
        obj.pop('Messages', None)  # Remove 'Messages' if present
        obj.pop('Labels', None)    # Remove 'Labels' if present
        for key, value in obj.items():
            remove_fields(value)  # Recursively remove from nested dictionaries
    elif isinstance(obj, list):
        for item in obj:
            remove_fields(item)    # Recursively remove from list items

# Function to convert a JSON object to base64 after removing certain fields
def json_to_base64(obj):
    # First, remove 'Messages' and 'Labels'
    obj_copy = json.loads(json.dumps(obj))  # Deep copy to avoid mutating the original
    remove_fields(obj_copy)
    json_bytes = json.dumps(obj_copy).encode('utf-8')
    return base64.b64encode(json_bytes).decode('utf-8')

# # Function to convert a JSON object to base64
# def json_to_base64(obj):
#     json_bytes = json.dumps(obj).encode('utf-8')
#     return base64.b64encode(json_bytes).decode('utf-8')

# Function to create JSON Diff URL
def create_diff_url(left_base64, right_base64):
    return f"https://jsondiff.com/#left=data:base64,{urllib.parse.quote(left_base64)}&right=data:base64,{urllib.parse.quote(right_base64)}"

# Function to write a single row in HTML
def write_row(html_file, row_num, name, node_name, diff_url, yield_diff_url):
    # Check if name equals node_name, and if so, set node_name to empty
    if name == node_name:
        node_name = ""
        # Write row number, name, and node_name to the HTML table
    html_file.write(f"<tr><td>{row_num}</td><td>{name}</td><td>{node_name}</td>")

    # Conditionally write the diff_url cell
    if diff_url:
        html_file.write(f"<td><a href=\"{diff_url}\" target=\"_blank\">Show diff</a></td>")
    else:
        html_file.write("<td></td>")

    # Conditionally write the yield_diff_url cell
    if yield_diff_url:
        html_file.write(f"<td><a href=\"{yield_diff_url}\" target=\"_blank\">Show yield diff</a></td>")
    else:
        html_file.write("<td></td>")

    html_file.write("</tr>\n")

# Main function
def main(json_file_path):
    # Read the JSON file
    with open(json_file_path, 'r') as f:
        objects = json.load(f)

    if len(objects) < 1:
        print("The JSON array must contain at least one object.")
        return

    # Variables to track the last yield node
    last_yield_obj = None
    last_yield_index = None

    # Open the HTML file
    with open('output.html', 'w') as html_file:
        # Write the header of the HTML file
        html_file.write("""
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>JSON Diff Results</title>
</head>
<body>
    <h1>JSON Diff Comparison</h1>
    <table border="1">
        <tr>
            <th>Row</th>
            <th>Name</th>
            <th>Node.Name</th>
            <th style="min-width:6em; text-align:center;">Diff Link</th>
            <th style="min-width:6em; text-align:center;">Yield Diff</th>
        </tr>
""")

        # Process the first object (0th element) separately
        first_obj = objects[0]
        if first_obj['Node']['name'] == 'yield':
            last_yield_obj = first_obj
            last_yield_index = 0
        write_row(html_file, 1, first_obj['Name'], first_obj['Node']['name'], "", "")

        # Iterate through remaining pairs of objects
        for i in range(1, len(objects)):
            left_obj = objects[i-1]
            right_obj = objects[i]

            # Convert both objects to base64
            left_base64 = json_to_base64(left_obj)
            right_base64 = json_to_base64(right_obj)

            # Create the JSON diff URL
            diff_url = create_diff_url(left_base64, right_base64)

            # Check if Node.name == 'yield' for this object
            yield_diff_url = ""
            if right_obj['Node']['name'] == 'yield':
                # If there's a previous yield object, create a diff link
                if last_yield_obj is not None:
                    last_yield_base64 = json_to_base64(last_yield_obj)
                    yield_diff_url = create_diff_url(last_yield_base64, right_base64)

                # Update last yield object and index
                last_yield_obj = right_obj
                last_yield_index = i

            # Write the row to the HTML file
            write_row(html_file, i + 1, right_obj['Name'], right_obj['Node']['name'], diff_url, yield_diff_url)

        # Write the footer of the HTML file
        html_file.write("""
    </table>
</body>
</html>
""")

if __name__ == '__main__':
    if len(sys.argv) < 2:
        print(f"Usage: {sys.argv[0]} <json-file>")
        sys.exit(1)

    json_file_path = sys.argv[1]
    main(json_file_path)

import os
import re

def normalize_basename(path):
    """Convert a file path to a normalized base name without extension."""
    name = os.path.splitext(os.path.basename(path))[0]
    # Convert camelCase/PascalCase to snake_case
    name = re.sub(r'([a-z0-9])([A-Z])', r'\1_\2', name)
    name = re.sub(r'([A-Z]+)([A-Z][a-z])', r'\1_\2', name)  # handle acronyms
    name = re.sub(r'[^0-9a-zA-Z_]', '_', name)
    name = re.sub(r'_+', '_', name)
    return name.strip('_').lower()

def pascal_case(name):
    """Convert snake_case or kebab-case to PascalCase."""
    return ''.join(word.capitalize() for word in re.split(r'[_\-]+', name))

def base_pascal_case(path):
    """Convert snake_case or kebab-case to PascalCase with a base prefix."""
    base = normalize_basename(path)
    return pascal_case(base)

def go_filenames(path, suffixes):
    base = normalize_basename(path)
    return [f"{base}{suffix}.go" for suffix in suffixes]

def python_filenames(path, suffixes):
    base = normalize_basename(path)
    return [f"{base}{suffix}.py" for suffix in suffixes]

def java_filenames(path, suffixes):
    base_pascal = pascal_case(normalize_basename(path))
    return [f"{base_pascal}{pascal_case(suffix)}.java" for suffix in suffixes]

def rust_filenames(path, suffixes):
    base = normalize_basename(path)
    return [f"{base}{suffix}.rs" for suffix in suffixes]

def path_for_java_package(java_package):
    return java_package.replace('.', os.sep)
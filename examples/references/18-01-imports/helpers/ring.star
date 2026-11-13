# A plain Starlark helper module, imported by Imports.fizz.
SIZE = 3

def next_slot(i):
    return (i + 1) % SIZE

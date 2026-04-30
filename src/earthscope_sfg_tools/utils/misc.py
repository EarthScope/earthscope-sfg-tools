def listify(value):
    if isinstance(value, list):
        return value
    else:
        return [value]

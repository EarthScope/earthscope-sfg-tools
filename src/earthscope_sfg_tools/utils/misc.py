def listify(value):
    """Wrap a non-list value in a list, or return it unchanged.

    Args:
        value: Any value. If already a list, returned as-is.

    Returns:
        The original list, or ``[value]`` for any non-list input.
    """
    if isinstance(value, list):
        return value
    else:
        return [value]

"""Logging utilities for earthscope_sfg_tools."""

from __future__ import annotations

import logging
from typing import Optional


def _build_logger(name: str) -> logging.Logger:
    """Build a logger with a default console handler."""
    logger = logging.getLogger(name)
    if not logger.handlers:
        handler = logging.StreamHandler()
        handler.setFormatter(logging.Formatter("%(asctime)s - %(levelname)s - %(message)s"))
        logger.addHandler(handler)
    logger.setLevel(logging.INFO)
    return logger


class _LoggerAdapter:
    """Adapter exposing legacy-style log method names."""

    def __init__(self, logger: logging.Logger) -> None:
        self._logger = logger

    def logdebug(self, message: str) -> None:
        self._logger.debug(message)

    def loginfo(self, message: str) -> None:
        self._logger.info(message)

    def logwarn(self, message: str) -> None:
        self._logger.warning(message)

    def logerr(self, message: str) -> None:
        self._logger.error(message)


# Global default logger instance
ProcessLogger = _LoggerAdapter(_build_logger("earthscope_sfg_tools.processing"))

# Global injection point for logger override
_injected_logger: Optional[_LoggerAdapter] = None


def set_logger(logger: logging.Logger | _LoggerAdapter | None) -> None:
    """Set a custom logger instance globally.

    Parameters
    ----------
    logger : logging.Logger | _LoggerAdapter | None
        A Python logger, adapter, or None to reset to default.
    """
    global _injected_logger
    if logger is None:
        _injected_logger = None
    elif isinstance(logger, _LoggerAdapter):
        _injected_logger = logger
    elif isinstance(logger, logging.Logger):
        _injected_logger = _LoggerAdapter(logger)
    else:
        raise TypeError(f"Expected logging.Logger, _LoggerAdapter, or None; got {type(logger)}")


def get_logger() -> _LoggerAdapter:
    """Get the current logger instance (injected or default).

    Returns
    -------
    _LoggerAdapter
        The active logger adapter.
    """
    return _injected_logger if _injected_logger is not None else ProcessLogger

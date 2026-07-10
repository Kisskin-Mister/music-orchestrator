from __future__ import annotations

from typing import Annotated

from fastapi import Depends, Header, HTTPException, Request, status
from fastapi.responses import JSONResponse

from .config import Settings, get_settings

USER_ID = "local-user"


def error_response(status_code: int, code: str, message: str, details: dict | None = None) -> JSONResponse:
    return JSONResponse(status_code=status_code, content={"error": {"code": code, "message": message, "details": details}})


def require_api_key(
    x_api_key: Annotated[str | None, Header(alias="X-API-Key")] = None,
    settings: Settings = Depends(get_settings),
) -> str:
    if not x_api_key or x_api_key not in settings.api_key_set:
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="Invalid or missing API key")
    return USER_ID


async def http_exception_handler(request: Request, exc: HTTPException) -> JSONResponse:
    code = "unauthorized" if exc.status_code == 401 else "http_error"
    return error_response(exc.status_code, code, str(exc.detail))

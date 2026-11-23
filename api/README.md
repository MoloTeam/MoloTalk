# РљРѕРЅС‚СЂР°РєС‚С‹ API

Proto вЂ” РµРґРёРЅСЃС‚РІРµРЅРЅС‹Р№ РёСЃС‚РѕС‡РЅРёРє С‚РёРїРѕРІ Рё РїСЂР°РІРёР». Go, TypeScript Рё OpenAPI вЂ”
РїСЂРѕРёР·РІРѕРґРЅС‹Рµ С„Р°Р№Р»С‹, РѕРЅРё РєРѕРјРјРёС‚СЏС‚СЃСЏ РєР°Рє golden Рё РїСЂРѕРІРµСЂСЏСЋС‚СЃСЏ РїРѕРІС‚РѕСЂРЅРѕР№
РіРµРЅРµСЂР°С†РёРµР№.

## РЎС‚СЂСѓРєС‚СѓСЂР°

```text
api/
  proto/molotalk/v1/            РїСѓР±Р»РёС‡РЅС‹Рµ RPC Рё Gateway
  proto/molotalk/internal/v1/   РІРЅСѓС‚СЂРµРЅРЅРёРµ RPC
  third_party/proto/            Р·Р°РєРѕРјРјРёС‡РµРЅРЅС‹Рµ РІРЅРµС€РЅРёРµ proto (buf.validate, google.api)
  openapi/base.public.yaml      security Рё РёСЃРєР»СЋС‡РµРЅРёСЏ Р±РµР· access token
  openapi/base.internal.yaml    СЃР»СѓР¶РµР±РЅС‹Р№ Bearer
  gen/openapi/public.json       golden РїСѓР±Р»РёС‡РЅРѕРіРѕ OpenAPI
  gen/openapi/internal.json     golden РІРЅСѓС‚СЂРµРЅРЅРµРіРѕ OpenAPI
  src/gen/                      TypeScript С‚РѕР»СЊРєРѕ РїСѓР±Р»РёС‡РЅРѕРіРѕ РїР°РєРµС‚Р°
```

`proto/materials/` СЃРѕРґРµСЂР¶РёС‚ СЃРЅРёРјРєРё РёСЃС…РѕРґРЅС‹С… РјР°С‚РµСЂРёР°Р»РѕРІ HLD, РїРѕ РєРѕС‚РѕСЂС‹Рј
РїРѕСЃС‚СЂРѕРµРЅ РєРѕРЅС‚СЂР°РєС‚: `openapi.json` (`17eedbb8вЂ¦`) Рё `gateway.proto` (`f90e96aeвЂ¦`).

## Р’РЅРµС€РЅРёРµ Р·Р°РІРёСЃРёРјРѕСЃС‚Рё

Р РµРµСЃС‚СЂ Buf РІ РѕРєСЂСѓР¶РµРЅРёРё РЅРµРґРѕСЃС‚СѓРїРµРЅ, РїРѕСЌС‚РѕРјСѓ РІРЅРµС€РЅРёРµ proto Р·Р°РєРѕРјРјРёС‡РµРЅС‹ РІ
[`third_party/`](third_party/README.md) СЃ Р·Р°РєСЂРµРїР»С‘РЅРЅС‹Рј РїСЂРѕРёСЃС…РѕР¶РґРµРЅРёРµРј, Р»РёС†РµРЅР·РёСЏРјРё
Рё SHA-256. `buf.lock` РЅРµ РёСЃРїРѕР»СЊР·СѓРµС‚СЃСЏ: СѓРґР°Р»С‘РЅРЅС‹С… Р·Р°РІРёСЃРёРјРѕСЃС‚РµР№ РЅРµС‚.

## РРЅСЃС‚СЂСѓРјРµРЅС‚С‹

| РРЅСЃС‚СЂСѓРјРµРЅС‚ | Р’РµСЂСЃРёСЏ | Р›РёС†РµРЅР·РёСЏ |
|---|---|---|
| `github.com/bufbuild/buf/cmd/buf` | v1.75.0 | Apache-2.0 |
| `google.golang.org/protobuf/cmd/protoc-gen-go` | v1.36.13-0.20261016172727-e9ee2a76ab8e | BSD-3-Clause |
| `connectrpc.com/connect/v2/cmd/protoc-gen-connect-go` | v2.0.2 | Apache-2.0 |
| `connectrpc.com/protoc-gen-connect-openapi` | v0.30.0 | MIT |
| `buf.build/go/protovalidate` | v1.4.1 | Apache-2.0 |
| `connectrpc.com/validate` | v0.9.0 | Apache-2.0 |
| `@bufbuild/protoc-gen-es`, `@bufbuild/protobuf` | 2.17.0 | Apache-2.0 |
| `@bufbuild/protovalidate` | 1.3.0 | Apache-2.0 |
| `@connectrpc/connect`, `@connectrpc/connect-web` | 2.3.0 | Apache-2.0 |

Go-РёРЅСЃС‚СЂСѓРјРµРЅС‚С‹ Р·Р°РєСЂРµРїР»РµРЅС‹ РґРёСЂРµРєС‚РёРІР°РјРё `tool` РІ `backend/go.mod`, JS вЂ” С‚РѕС‡РЅС‹РјРё
РІРµСЂСЃРёСЏРјРё Рё `package-lock.json`. Р“Р»РѕР±Р°Р»СЊРЅР°СЏ СѓСЃС‚Р°РЅРѕРІРєР° РїР»Р°РіРёРЅРѕРІ, `protoc` Рё Docker
РЅРµ С‚СЂРµР±СѓСЋС‚СЃСЏ.

## РљРѕРјР°РЅРґС‹

РР· РєРѕСЂРЅСЏ СЂРµРїРѕР·РёС‚РѕСЂРёСЏ:

```bash
npm --prefix api ci
npm --prefix api run generate
npm --prefix api run check
```

Р“РµРЅРµСЂР°С†РёСЏ РѕРіСЂР°РЅРёС‡РµРЅР° РєР°С‚Р°Р»РѕРіР°РјРё РєРѕРЅС‚СЂР°РєС‚РѕРІ: Go РїРѕР»СѓС‡Р°РµС‚ РѕР±Р° РїР°РєРµС‚Р°, TypeScript вЂ”
С‚РѕР»СЊРєРѕ `proto/molotalk/v1`, OpenAPI СЃРѕР±РёСЂР°РµС‚СЃСЏ РѕС‚РґРµР»СЊРЅРѕ РґР»СЏ РїСѓР±Р»РёС‡РЅРѕРіРѕ Рё
РІРЅСѓС‚СЂРµРЅРЅРµРіРѕ РїР°РєРµС‚РѕРІ.

РћР±РЅРѕРІР»РµРЅРёРµ golden РІС…РѕРґРёС‚ РІ СЂРµРІСЊСЋ РёР·РјРµРЅРµРЅРёСЏ proto РёР»Рё base-С„Р°Р№Р»Р° Рё РЅРµ РІС‹РїРѕР»РЅСЏРµС‚СЃСЏ
РєРѕРјР°РЅРґРѕР№ `check` Р°РІС‚РѕРјР°С‚РёС‡РµСЃРєРё.

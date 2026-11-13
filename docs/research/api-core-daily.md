# Bilibili web core and Daily protocol specification

**Source baseline:** BiliBiliToolPro `main`, tree SHA `4ab02ce28cae854a3d84c713a3ccb483006b8213`. Protocol facts were extracted; no implementation code was copied or executed.

**Documentation availability:** SocialSisterYi/bilibili-API-collect now resolves to a repository whose `deprecated` branch contains a shutdown notice and no API documentation. Its available history was also replaced. References below marked **BAC mirror** point to preserved copies of its `docs/` files. These are historical documentation, not evidence that an endpoint currently works. [Upstream notice](https://github.com/SocialSisterYi/bilibili-API-collect/blob/deprecated/README.md).

**Notation:** `code` means JSON `$.code`, not HTTP status. `Form` means `application/x-www-form-urlencoded`. Cookie-authenticated operations use `SESSDATA`; mutation requests generally send `csrf=<bili_jct>`. Exact server enforcement of individual browser headers is **[UNCONFIRMED]** unless explicitly documented. Header values described as “BTP sends” are confirmed client behavior.

## 1. Login

### 1.1 Web QR login

1. Generate a QR code.
2. Render the returned `$.data.url` as a QR image; retain `$.data.qrcode_key`.
3. Poll until confirmed, expired, or cancelled.
4. On confirmed login, collect **all `Set-Cookie` headers** and retain `$.data.refresh_token` separately from cookies.
5. Enrich device cookies as described in §4; persist account credentials.

| Endpoint | Authentication, headers, parameters | Relevant response and codes |
|---|---|---|
| **GET `https://passport.bilibili.com/x/passport-login/web/qrcode/generate`** | No existing account authentication or signing documented. Browser UA. No request parameters in BTP. Specific required Referer/Origin: **[UNCONFIRMED]**. | `$.code=0`; `$.data.url`: QR contents; `$.data.qrcode_key`: polling key. BAC documents a **180-second** lifetime. |
| **GET `https://passport.bilibili.com/x/passport-login/web/qrcode/poll`** | Query: `qrcode_key` required. BTP additionally sends `source=main_mini`. No APP/WBI signature. Browser UA; specific required Referer/Origin **[UNCONFIRMED]**. | Outer `$.code=0` means the polling request succeeded. **`$.data.code`**: `86101` waiting for scan; `86090` scanned, awaiting confirmation; `86038` expired; `0` authenticated. `$.data.message`: status description; `$.data.url`: cross-domain login URL; `$.data.timestamp`: login time, Unix milliseconds; `$.data.refresh_token`: web cookie refresh credential. |

**Returned cookies:** `SESSDATA`, `bili_jct`, `DedeUserID`, `DedeUserID__ckMd5`; the documented HTTP capture also includes `sid`. Preserve cookie attributes in a cookie jar. `refresh_token` is a JSON field, **not a cookie**; the browser stores its equivalent as localStorage `ac_time_value`.

**BTP `Login` behavior:**

- `LoginTaskAppService`: QR login → homepage/device-cookie enrichment → persistence.
- Console QR login explicitly waits **5 seconds before each of at most 10 polls**. The global request interval can add further delay.
- HTTP polling failure: log and continue; invalid outer JSON code/data: stop; `data.code=86038`: stop; `data.code=0`: parse cookies, validate their structural completeness, return.
- `BiliCookie.Check()` requires a numeric `DedeUserID`, nonempty `SESSDATA`, and nonempty `bili_jct`.
- `TokenDto` contains `Refresh_token`, but the inspected login paths **do not persist or otherwise consume it**. BTP therefore does not provide the complete web refresh lifecycle in §3.
- Persistence targets QingLong, Baihu, or DaiDai environment variables when configured; otherwise local JSON.
- Separate web-dialog generation/poll methods exist and use the same web QR endpoints.

Sources: [IPassportApi.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Interfaces/IPassportApi.cs), [LoginDomainService.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.DomainService/LoginDomainService.cs), [LoginTaskAppService.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Application/LoginTaskAppService.cs), [TokenDto.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/PassportApi/TokenDto.cs), [BiliCookie.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliCookie.cs), [BAC mirror `docs/login/login_action/QR.md`](https://github.com/bilibili-plugins/bilibili-api-collect/blob/master/docs/login/login_action/QR.md).

### 1.2 TV QR login yielding APP credentials

**Current BTP runtime:** the inspected registered task clients use cookies. Its `Login` task does **not** perform TV QR login. No current Daily, Silver2Coin, or live sign-in operation inspected requires `access_token`. APP login/signing requests exist in the **Bruno collection**, which is separate from runtime task authentication. `BaseAppRequest` has `access_key` and `appkey` commented out; the `configApp` UA configuration is declared but not used by the listed runtime client registrations.

Optional TV login protocol:

1. Choose a compatible APPKEY/APPSEC pair.
2. Sign and submit QR generation.
3. Render returned `url`; retain `auth_code`.
4. Sign each poll request.
5. Persist returned APP token, APP refresh token, expiry, and any returned cookies.

| Endpoint | Request | Response and known codes |
|---|---|---|
| **POST `https://passport.bilibili.com/x/passport-tv-login/qrcode/auth_code`** | Form: `appkey`, `local_id=0`, `ts=<Unix seconds>`, `sign`. Optional `mobi_app`. APP-key signature; no existing login required. Exact UA/Referer/Origin enforcement **[UNCONFIRMED]**. | `$.data.url`, `$.data.auth_code`; QR lifetime **180 seconds**. `0` success; `-3` invalid API signing key/signature; `-400` bad request. |
| **POST `https://passport.bilibili.com/x/passport-tv-login/qrcode/poll`** | Form: `appkey`, `auth_code`, `local_id=0`, `ts`, `sign`. Same signing-key family as generation. | `$.data.mid`, `access_token`, `refresh_token`, `expires_in`. Sample also contains `$.data.token_info.*`, `$.data.cookie_info.cookies[]` with `name/value/expires/http_only/secure`, and `$.data.sso[]`. `0` success; `-3` bad signing key; `-400` bad request; `-404` absent; `86038` expired; `86039` not confirmed; `86090` scanned, awaiting confirmation. Documented default token lifetime: `15552000` seconds. |

BAC also lists equivalent hosts:

- `https://passport.snm0516.aisee.tv/x/passport-tv-login/qrcode/auth_code`
- `https://passport.snm0516.aisee.tv/x/passport-tv-login/qrcode/poll`

**Documented TV pair:** `appkey=4409e2ce8ffd12b8`, `appsec=59b43e04ad6965f34319062b478f83dd`.

**Other documented compatible pairs relevant to BTP’s Bruno examples:**

| Purpose | APPKEY | APPSEC |
|---|---|---|
| Android login/user information | `783bbb7264451d82` | `2653583c8873dea268ab9386918b1d65` |
| Android API requests | `1d8b6e7d45233436` | `560c52ccd288fed045859ed18bffd973` |

**APP signing:** remove existing `sign`; add `appkey` and endpoint-required `ts`; sort parameter keys ascending; URL-query serialize all parameters; append APPSEC directly; MD5 the UTF-8 bytes; use 32-character lowercase hex as `sign`. BTP Bruno uses percent encoding via `encodeURIComponent`; BAC examples also use form-query serializers. Space/special-character serialization must match the signed request; those examples differ in serializer choice.

The returned `access_token` is subsequently supplied as **`access_key`** to APP endpoints. BAC warns that tokens generally require the same key pair used to issue them, otherwise `-663` authentication failure. It explicitly documents the Android login pair above as an exception usable with the Android API pair.

Sources: [BAC mirror QR.md](https://github.com/bilibili-plugins/bilibili-api-collect/blob/master/docs/login/login_action/QR.md), [BAC mirror `docs/misc/sign/APP.md`](https://github.com/bilibili-plugins/bilibili-api-collect/blob/master/docs/misc/sign/APP.md), [Bruno `lib/sign.js`](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/bruno/lib/sign.js), [Bruno environment](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/bruno/environments/default.yml), [Bruno APP login example](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/bruno/passport.bilibili.com/x/passport-login/oauth2/login.yml), [BaseAppRequest.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/BaseAppRequest.cs), [client registrations](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/Extensions/ServiceCollectionExtension.cs).

## 2. Cookie validity, account information, and daily state

1. Validate the account using `nav`.
2. Read daily watch/share state using `exp/reward`.
3. Before coin spending, obtain live coin balance and the separate coin-experience total.
4. Treat account-authentication failure separately from transport/risk-control failure.

| Endpoint | Auth, BTP headers, parameters | Relevant response and codes |
|---|---|---|
| **GET `https://api.bilibili.com/x/web-interface/nav`** | Cookie `SESSDATA`; browser UA; Referer `https://www.bilibili.com/`; Origin `https://www.bilibili.com`. No query, CSRF or WBI required. | `0` authenticated; `-101` unauthenticated. Require `$.data.isLogin=true` for validity. WBI image fields can exist even when unauthenticated. |
| **GET `https://account.bilibili.com/site/getCoin`** | Cookie `SESSDATA`; BAC additionally requires nonzero `DedeUserID`. Browser UA; Referer `https://account.bilibili.com/account/coin`. No query, CSRF or WBI. | `$.data.money`: decimal coin balance; BAC documents `null` when zero. `$.status` also exists. `0` success; `-101` unauthenticated. BTP treats missing money on a valid response as zero. |
| **GET `https://api.bilibili.com/x/member/web/exp/reward`** | Cookie `SESSDATA`. Referer `https://account.bilibili.com/account/home`; Origin `https://account.bilibili.com`; browser UA. No query/CSRF/WBI in BTP. | `$.data.login`, `watch`, `share`: booleans, each daily completion rewards 5 exp. **`$.data.coins` is experience, not coin count**, maximum 50; BAC documents delayed updates. `0` success; `-101` unauthenticated. |
| **GET `https://api.bilibili.com/x/web-interface/coin/today/exp`** | Cookie `SESSDATA`; browser UA; Referer `https://www.bilibili.com/`; Origin `https://www.bilibili.com`. No query/CSRF/WBI. | **`$.data` is an integer experience total**, capped at 50. BAC documents realtime updates. `0` success; other endpoint-specific error codes **[UNCONFIRMED]**. BTP rejects any nonzero code. |

`nav` fields:

| JSON path | Meaning |
|---|---|
| `$.data.mid` | Account UID |
| `$.data.uname` | Display name |
| `$.data.face` | Avatar URL |
| `$.data.level_info.current_level` | Current account level |
| `$.data.level_info.current_exp` | Current experience |
| `$.data.level_info.next_exp` | Next level’s total-exp threshold; number below Lv6, string `"--"` at Lv6 |
| `$.data.money` | Coin balance; decimal |
| `$.data.vipStatus` | `0` inactive, `1` active |
| `$.data.vipType` | `0` none, `1` monthly, `2` annual or above; BTP only considers this meaningful when status is active |
| `$.data.vipDueDate` | VIP expiry, Unix **milliseconds** |
| `$.data.vip.due_date` | Also present in the BAC example, Unix milliseconds |
| `$.data.wallet.bcoin_balance` | B-coin balance, distinct from coins |
| `$.data.wallet.coupon_balance` | Monthly B-coin coupon balance |
| `$.data.wbi_img.img_url` / `sub_url` | WBI key carrier URLs |

**BTP behavior:**

- Daily order: cookie enrichment → `nav` login → reward status → watch/share → coin donation → VIP privilege claiming.
- Reward-status nonzero code causes **one additional request**; missing data falls back to an all-false/default status object.
- `CoinDomainService` calculates credited coins as `coinTodayExp / 10` using integer division.
- Failed coin-experience or balance precondition queries cause the coin stage to be skipped.
- `CookieTaskGuard` checks once per local day and whenever the cookie fingerprint changes, unless explicitly forced. `-101` or successful `isLogin=false` means invalid; other errors mean unconfirmed state and skip the activity, not proof of expiry.

Sources: [INavApi.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Interfaces/INavApi.cs), [IAccountApi.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Interfaces/IAccountApi.cs), [IApiApi.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Interfaces/IApiApi.cs), [AccountDomainService.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.DomainService/AccountDomainService.cs), [CoinDomainService.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.DomainService/CoinDomainService.cs), [CookieTaskGuard.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Application/CookieTaskGuard.cs), [BAC mirror login_info.md](https://github.com/bilibili-plugins/bilibili-api-collect/blob/master/docs/login/login_info.md), [BAC mirror member_center.md](https://github.com/bilibili-plugins/bilibili-api-collect/blob/master/docs/login/member_center.md).

## 3. Web cookie refresh

**This is BAC’s documented flow; it is not implemented by the inspected BTP Login service.**

1. On the first activity of the day, check whether refresh is needed.
2. Generate `correspondPath` using RSA-OAEP.
3. Fetch the corresponding HTML page and extract `refresh_csrf`.
4. Retain the **old** refresh token.
5. Refresh using old cookies and old token; capture new cookies and new token.
6. Confirm using **new cookies, new `bili_jct`, and the old refresh token**.
7. BAC’s flow then performs SSO cross-domain login.

| Endpoint | Authentication and request | Response and codes |
|---|---|---|
| **GET `https://passport.bilibili.com/x/passport-login/web/cookie/info`** | Existing cookie `SESSDATA`; optional query `csrf=<bili_jct>`. No APP/WBI signature. Specific required UA/Referer/Origin **[UNCONFIRMED]**. | `$.data.refresh`: boolean; `$.data.timestamp`: server Unix milliseconds, usable for correspondPath. `0` success; `-101` unauthenticated. |
| **GET `https://www.bilibili.com/correspond/1/{correspondPath}`** | Existing cookies. Path is the encrypted lowercase-hex value below. Specific header requirements **[UNCONFIRMED]**. | HTML: extract text of **`div#1-name`** as `refresh_csrf`. Bad/expired path returns HTTP **404**, not a JSON business code. |
| **POST `https://passport.bilibili.com/x/passport-login/web/cookie/refresh`** | Existing cookies. Form: `csrf=<old bili_jct>`, `refresh_csrf=<HTML token>`, `source=main_web`, `refresh_token=<old web refresh token>`. | New `Set-Cookie`: `sid`, `DedeUserID`, `DedeUserID__ckMd5`, `SESSDATA`, `bili_jct`. JSON `$.data.status=0`, `$.data.message`, `$.data.refresh_token=<new token>`. Codes: `0`; `-101`; `-111` CSRF invalid; `86095` invalid refresh_csrf or refresh_token/cookie mismatch. |
| **POST `https://passport.bilibili.com/x/passport-login/web/confirm/refresh`** | **New cookies**. Form: `csrf=<new bili_jct>`, `refresh_token=<old token>`. | Invalidates the cookie corresponding to the old token. Codes: `0`; `-101`; `-111`; `-400` bad request. |

**`correspondPath` construction:**

- Plaintext: UTF-8 bytes of `refresh_` followed by the decimal Unix **millisecond** timestamp.
- RSA-OAEP, hash **SHA-256**, MGF1 **SHA-256**, empty label.
- Encode the ciphertext as lowercase hexadecimal.
- Published protocol public key:

```text
-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDLgd2OAkcGVtoE3ThUREbio0Eg
Uc/prcajMKXvkCKFCWhJYJcLkcM2DKKcSeFpD/j6Boy538YXnR6VhcuUJOhH2x71
nzPjfdTcqMz7djHum0qSZA0AyCBDABUqCrfNgCiJ00Ra7GmRj+YCK1NJEuewlb40
JNrRuoEUXpabUzGB8QIDAQAB
-----END PUBLIC KEY-----
```

A manually imported cookie without its original refresh token is insufficient to perform this flow. Web and APP refresh tokens must be kept distinct.

Sources: [BAC mirror `docs/login/cookie_refresh.md`](https://github.com/bilibili-plugins/bilibili-api-collect/blob/master/docs/login/cookie_refresh.md), [BTP passport interface](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Interfaces/IPassportApi.cs), [BTP login service](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.DomainService/LoginDomainService.cs).

## 4. WBI and device cookies

### 4.1 WBI signing flow

1. Obtain `img_url` and `sub_url` from `nav`.
2. Extract each final filename without `.png`; concatenate `img_key + sub_key`.
3. Permute characters using the table below and take the first **32** characters as `mixin_key`.
4. Remove previous `w_rid` and `wts`; insert `wts=<current Unix seconds>`.
5. Remove characters **`!'()*`** from parameter values.
6. Sort keys ascending; percent-encode keys and values; join as `key=value&...`. Use uppercase hexadecimal escapes and **`%20` for spaces**.
7. Compute lowercase hex `MD5(UTF8(canonical_query + mixin_key))` as `w_rid`.
8. Send original endpoint parameters plus `wts` and `w_rid`.

```text
46,47,18,2,53,8,23,32,15,50,10,31,58,3,45,35,
27,43,5,49,33,9,42,19,29,28,14,39,12,38,41,13,
37,48,7,16,24,55,40,61,26,17,0,1,60,51,30,4,
22,25,54,21,56,59,6,63,57,62,11,36,20,34,44,52
```

BTP signs requests whose parameters contain the `wts` marker. Its WBI service caches keys per `BiliCookie` object without an explicit expiry in the inspected implementation. BAC says keys are observed to change daily. BAC also warns that an invalid signature may return `code=0` with `$.data.v_voucher` instead of the requested data.

| Endpoint within this assignment | WBI status |
|---|---|
| `/x/space/wbi/arc/search` | BTP signs; required for the selected web search route |
| `/x/web-interface/wbi/view` | BTP signs; current source explicitly moved from unsigned `/view` to avoid `-412` |
| `/x/web-interface/ranking/v2` | BTP does not sign; BAC documents WBI parameters as optional |
| `/x/click-interface/web/heartbeat` | BTP does not sign; BAC documents WBI query parameters as optional |
| QR, refresh, nav, coin balance, reward status, coin-exp status, relation lists, archive coin count, share, coin mutation, web like, live sign/exchange | No WBI in inspected BTP requests or cited protocol flows |

Sources: [WbiService.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Services/WbiService.cs), [WridEncryptionDelegatingHandler.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/WridEncryptionDelegatingHandler.cs), [GetVideoDetailRequest.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/ApiApi/Video/GetVideoDetailRequest.cs), [BAC mirror WBI.md](https://github.com/bilibili-plugins/bilibili-api-collect/blob/master/docs/misc/sign/wbi.md).

### 4.2 BTP device enrichment

1. **GET `https://www.bilibili.com/`** with the account cookie and browser UA; merge all `Set-Cookie` values.
2. If any pinned `DeviceCookie` values are configured, apply them and skip automatic enrollment.
3. Otherwise, preserve a device already containing nonempty `buvid3` and `_uuid`.
4. If `buvid3` is absent, request `finger/spi`.
5. For an unenrolled device, send a Gaia device report; persist `_uuid` only on `code=0`.

| Endpoint | Request and returned fields |
|---|---|
| **GET `https://www.bilibili.com/`** | Browser UA and existing cookies. BAC documents `Set-Cookie` containing `buvid3` and `b_nut`. No account login required to acquire device cookies. BAC warns against sensitive UA substrings such as `curl`/`python` and rapid repeated identical-UA requests. |
| **GET `https://api.bilibili.com/x/frontend/finger/spi`** | BTP sends existing cookie, Referer `https://www.bilibili.com/`, Origin `https://www.bilibili.com`; no WBI. `$.data.b_3` → cookie `buvid3`; `$.data.b_4` → cookie `buvid4`; `code=0` success. Other endpoint-specific codes **[UNCONFIRMED]**. |
| **POST `https://api.bilibili.com/x/internal/gaia-gateway/ExClimbWuzhi`** | JSON object with a **`payload` string containing serialized fingerprint JSON**. Cookie header contains device cookies `buvid3`, optional `buvid4`, optional `b_nut`, and `_uuid`; BTP deliberately builds this header without account cookies. Referer `https://www.bilibili.com/`; Origin `https://www.bilibili.com`; matching fixed fingerprint UA below. `code=0` considered enrolled; other codes are logged without replacing existing credentials. |

**Gaia UA used by current BTP:**

```text
Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/151.0.0.0 Safari/537.36 Edg/151.0.0.0
```

BTP’s report derives from a captured browser feature template. Dynamic fields are `5062` timestamp in milliseconds, `03bf` percent-encoded homepage Referer, `39c8=333.1007.fp.risk`, `df35=_uuid`; it also varies screen arrays and canvas tails. This is **observed BTP behavior**, not a confirmed universal Gaia schema. A clean-room client should not copy its captured template.

| Cookie/configuration | Confirmed BTP behavior |
|---|---|
| `buvid3` | Homepage response, or `finger/spi` if missing |
| `buvid4` | `finger/spi`; not regenerated solely because missing when `buvid3`+`_uuid` already exist |
| `b_nut` | Homepage `Set-Cookie`, existing imported value, or pinned config; `finger/spi` does not return it |
| `_uuid` | Existing value or generated browser-style identifier; serves as BTP’s enrollment marker |
| `DeviceCookie:Buvid3`, `Buvid4`, `BNut` | Empty by default; any nonempty pinned value disables automatic enrollment for that run |
| `bili_ticket` | No explicit generation logic in the inspected BTP device/login flow; imported or received cookies are preserved generically |

**Optional documented `bili_ticket` acquisition, not BTP runtime behavior:**

1. Let `ts` be Unix seconds.
2. `hexsign = lowercase_hex(HMAC-SHA256(key="XgwSnGZ1p", message="ts"+decimal(ts)))`.
3. **POST `https://api.bilibili.com/bapis/bilibili.api.ticket.v1.Ticket/GenWebTicket`** with URL parameters `key_id=ec02`, `hexsign`, `context[ts]=ts`, optional `csrf=<bili_jct or empty>`.
4. Referer may be empty or a `.bilibili.com` page. No APP/WBI signature.
5. Read `$.data.ticket`, `created_at`, `ttl`; retain as cookie `bili_ticket` with the returned lifetime. Response also includes `$.data.nav.img/sub` WBI carrier URLs.
6. `code=0` success; **positive `400`** parameter error.

BAC describes the ticket as optional and reducing risk-control probability. Its introduction says `259260` seconds, whereas its response table/example says **`259200` seconds**; use returned `ttl`.

Sources: [LoginDomainService.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.DomainService/LoginDomainService.cs), [IGaiaApi.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Interfaces/IGaiaApi.cs), [Gaia template source](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/GaiaDeviceFingerprintTemplate.cs), [BAC mirror buvid3_4.md](https://github.com/bilibili-plugins/bilibili-api-collect/blob/master/docs/misc/buvid3_4.md), [BAC mirror bili_ticket.md](https://github.com/bilibili-plugins/bilibili-api-collect/blob/master/docs/misc/sign/bili_ticket.md).

## 5. Daily task

### 5.1 Target video selection

**Watch/share selection actually implemented:**

1. Random configured `SupportUpIds` creator.
2. Otherwise random creator from the **first page of normal followings**.
3. Otherwise a random video from the whole-site ranking.

**Coin selection actually implemented:**

1. Configured creators: one selection attempt.
2. Special-follow group: one attempt.
3. Normal followings: one attempt.
4. Ranking: up to five attempts.
5. Repeat this selection process within the donation loop, up to **10 outer attempts**.

**Difference from documentation:** `configuration.md` describes special-follow priority generally for watch/share/coins. `VideoDomainService` does **not** query the special-follow group for watch/share; `DonateCoinDomainService` does.

| Endpoint | Auth, headers and query | Response, codes, use |
|---|---|---|
| **GET `https://api.bilibili.com/x/relation/tag`** | Account cookie; browser UA; Referer `https://space.bilibili.com/`; `Cache-Control: no-cache`, `Pragma: no-cache`. Query BTP: `mid=<own UID>`, `tagid=-10`, `pn=1`, `ps=20`, `jsonp=jsonp`. No WBI. | `$.data[]` creator objects including `mid`, `uname`. `0`; `-101`; `-400`; `22104` nonexistent group. Used for coin targets only. |
| **GET `https://api.bilibili.com/x/relation/followings`** | Account cookie; browser UA; Referer `https://space.bilibili.com/`. Query `vmid=<own UID>`, `order_type=attention`, `pn=1`, `ps=20`, `order=desc`, `jsonp=jsonp`. No WBI. | `$.data.total`, `$.data.list[].mid/uname`. `0`; `-101`; `-352` intercepted; `-400`; `22115` private following list. BTP does not paginate all followings for these selection paths. |
| **GET `https://api.bilibili.com/x/space/wbi/arc/search`** | Account cookie + WBI; browser UA; Referer `https://www.bilibili.com/`; Origin `https://space.bilibili.com`. Query: `mid`, `ps=30`, `pn=1`, `tid=0`, `keyword=""`, `order=pubdate`, `platform=web`, `web_location=1550101`, `order_avoided=true`, `wts`, `w_rid`. | `$.data.page.count`: total uploads; `$.data.list.vlist[].aid/bvid/title/length`. `0`; `-400`; `-412` intercepted. BTP first gets count, then sends **`ps=1`, random `pn` in `1..count`**. `length` is parsed as minutes:seconds. |
| **GET `https://api.bilibili.com/x/web-interface/ranking/v2?rid=0&type=all`** | No cookie parameter in BTP. Browser UA; Referer `https://www.bilibili.com/`; Origin `https://www.bilibili.com`; `dnt: 1`. No WBI in BTP; BAC says optional. | `$.data.list[].aid/bvid/cid/copyright/duration/title`. `0`; `-400`. BTP selects one random list item. This is its “region” fallback; current code uses whole-site `rid=0`, not separate region-scanning endpoints. |
| **GET `https://api.bilibili.com/x/web-interface/wbi/view`** | Account cookie + WBI; browser UA. Query BTP: `aid`, `wts`, `w_rid`. Its interface adds no specific Referer/Origin. | BTP consumes `$.data.copyright` and video details. Needed for original/repost coin limit. `0` expected. Source explicitly discusses `-412` on unsigned requests; full endpoint error set **[UNCONFIRMED]**. |

Coin target filtering:

- Exclude own UID, `0`, invalid parsed creator IDs, and creators with no videos.
- Cache upload counts per creator for the run.
- Do not reselect a video already checked/attempted in the run, whether the donation succeeds or fails.
- Check prior coin amount and copyright before donation.
- Watch/share creator selection does not implement the coin path’s full fallback-attempt handling.

Sources: [VideoDomainService.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.DomainService/VideoDomainService.cs), [DonateCoinDomainService.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.DomainService/DonateCoinDomainService.cs), [IApiApi.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Interfaces/IApiApi.cs), [search request fields](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/ApiApi/Video/SearchVideosByUpIdFullDto.cs), [following request](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/ApiApi/Relation/GetFollowingsRequest.cs), [special-follow request](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/ApiApi/Relation/GetSpecialFollowingsRequest.cs), [BAC mirror relation.md](https://github.com/bilibili-plugins/bilibili-api-collect/blob/master/docs/user/relation.md), [BAC mirror space.md](https://github.com/bilibili-plugins/bilibili-api-collect/blob/master/docs/user/space.md), [BAC mirror ranking.md](https://github.com/bilibili-plugins/bilibili-api-collect/blob/master/docs/video_ranking/ranking.md).

### 5.2 Watch video

1. Run only when `watch=false` and `IsWatchVideo=true`.
2. Send an opening heartbeat with time values zero.
3. Send another heartbeat with the three elapsed/progress values equal.
4. BTP logs success on `code=0`; it does not reread reward state here.

**POST `https://api.bilibili.com/x/click-interface/web/heartbeat`**

- Auth: account cookie, `csrf=<bili_jct>`; no APP token or WBI in BTP.
- Headers: browser UA; Referer `https://www.bilibili.com/`; Origin `https://www.bilibili.com`; `Content-Type: application/x-www-form-urlencoded; charset=UTF-8`.
- BTP duplicates `aid` and `played_time` in the **URL query** as well as body.
- Body fields:

| Field | BTP value / meaning |
|---|---|
| `aid`, `bvid` | Target identifiers |
| `cid` | Optional part/content identifier |
| `mid` | Own account UID |
| `csrf` | Cookie `bili_jct` |
| `played_time` | Current progress in seconds; initial `0` |
| `realtime` | Session elapsed play time; initial `0` |
| `real_played_time` | Actual video playback duration; initial `0` |
| `start_ts` | Playback start timestamp |
| `type` | `3` ordinary uploaded video |
| `dt` | `2` |
| `play_type` | BTP default **`3`**, including its opening request |
| `epid`, `sid`, `sub_type` | Nullable episode/season/subtype fields |

**BTP played-time rule:** unknown duration becomes 15; `max=min(duration,15)`; uses `Random.Next(1,max)`, so normally **1–14 seconds**, with the upper bound excluded. The same value is assigned to `played_time`, `realtime`, `real_played_time`. There is no separate playback sleep in this method; request middleware introduces spacing.

BAC documents `played_time=-1` as finished and conventional heartbeat spacing of **15 seconds**. BTP does not use `-1`. BAC explicitly labels some heartbeat parameter semantics as inferred. Creator-search results do not supply `cid` to BTP’s watch DTO; ranking results do.

Response: `code=0` success; `-400` request error; `message`. A successful heartbeat is not independently verified evidence of credited experience.

Sources: [VideoDomainService.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.DomainService/VideoDomainService.cs), [UploadVideoHeartbeatRequest.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/ApiApi/Video/UploadVideoHeartbeatRequest.cs), [BAC mirror report.md](https://github.com/bilibili-plugins/bilibili-api-collect/blob/master/docs/video/report.md).

### 5.3 Share video

1. Run only when `share=false` and `IsShareVideo=true`.
2. Reuse the watch target.
3. If watch did not run, send an opening heartbeat before sharing; BTP catches opening exceptions and still attempts share.
4. Submit share; `-403` ends this share attempt without retry or device refresh.

**POST `https://api.bilibili.com/x/web-interface/share/add`**

- Cookie: account session and valid `buvid3`; BTP always supplies CSRF.
- Headers: browser UA; Origin `https://www.bilibili.com`; Form. BTP adds no endpoint-specific Referer.
- Form: `aid`, `csrf`, **`eab_x=1`**, `ramval=<random integer 3..19>`, `source=web_normal`, `ga=1`.
- BAC also permits `bvid` instead of `aid`; documents optional `csrf`, `eab_x=2`, `ramval=0`. These differ from BTP’s submitted values.
- Response `$.data`: current share count according to BAC.
- Codes: `0`; `-101`; `-111`; `-400`; `71000` duplicate share.
- **Source disagreement:** BAC lists **positive `403`** for account abnormality; BTP specifically handles **negative `-403`**, describing it as an account-level risk marker and skipping today’s share.

Sources: [VideoDomainService.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.DomainService/VideoDomainService.cs), [ShareVideoRequest.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/ApiApi/Video/ShareVideoRequest.cs), [BAC mirror action.md](https://github.com/bilibili-plugins/bilibili-api-collect/blob/master/docs/video/action.md).

### 5.4 Add coins

1. Apply the per-account level stop rule before either video or article donation.
2. If configured target is nonpositive, skip.
3. Read today’s coin experience; calculate `already=exp/10`, `need=target-already`.
4. Read coin balance and reserve; reduce spending to available whole coins above reserve.
5. Select eligible videos using §5.1.
6. Query per-video coin count and copyright.
7. Donate **one coin per request**, optionally liking.
8. Stop after the required successes or 10 outer attempts; query balance again.

| Endpoint | Request | Response and codes |
|---|---|---|
| **GET `https://api.bilibili.com/x/web-interface/archive/coins`** | Account cookie; browser UA; Referer `https://www.bilibili.com/`. Query BTP `aid`; BAC also permits `bvid`. No CSRF/WBI. | `$.data.multiply`: already donated amount, `0` none. `0`; `-101`; `-400`. |
| **POST `https://api.bilibili.com/x/web-interface/coin/add`** | Account cookie including valid `buvid3`; Form; browser UA; Origin `https://www.bilibili.com`; target-video Referer. Body below. | `$.data.like`: whether accompanying like succeeded; already liked can yield false while coin donation succeeds. Codes below. |

BTP coin form:

```text
aid=<video aid>
multiply=1
select_like=<1 if configured, otherwise 0>
cross_domain=true
csrf=<bili_jct>
eab_x=2
ramval=3
source=web_normal
ga=1
```

BTP Referer shape:

```text
https://www.bilibili.com/video/{bvid}/?spm_id_from=333.1007.tianma.1-1-1.click&vd_source=80c1601a7003934e7a90709c18dfcffd
```

BAC permits `aid` or `bvid`; `multiply` has per-video/request upper bound 2. BTP checks original uploads (`copyright=1`) permit 2 coins total; other copyright values permit 1. A prior count already at the limit excludes the video.

| Code | Meaning | BTP coin behavior |
|---|---|---|
| `0` | Donation succeeded | Increment successful requests by one |
| `-101` | Not logged in | Abort coin stage |
| `-102` | Account banned | Abort |
| `-104` | Insufficient coins | Abort |
| `-111` | CSRF invalid | Abort |
| `-403` | Account abnormal | Abort |
| `-400` | Bad request | May continue with another candidate |
| `10003` | Video absent | Continue |
| `34002` | Own video | Continue |
| `34003` | Invalid quantity | Continue |
| `34004` | Donation interval too short | Continue |
| `34005` | Per-video coin limit reached | Continue |
| Other nonzero codes, including `-352`/`-412` | Unanticipated failure | Throw and terminate coin flow |

**Daily cap distinction:** BAC documents a **50-exp daily reward cap**, equivalent to five credited coins. This does not establish a server-wide prohibition on spending more than five coins. BTP docs and UI say target `0–5`; however, current `GetNeedDonateCoinNum` assigns the raw configured target, and its `MaxNumberOfDonateCoins` clamp is **commented out**. The constant still equals 5. A value above 5 supplied outside the UI is therefore not safely clamped by that method.

**Reserve:** skip when balance `<=0` or `<=NumberOfProtectedCoins`. Spend at most the truncated whole-coin balance remaining above reserve.

**Level stop:** effective threshold is `CoinDonationStopLevel` when positive; otherwise 6 if `SaveCoinsWhenLv6=true`; otherwise disabled. Skip donation when the account’s current level is at or above that threshold. Watch/share continue.

Sources: [DonateCoinDomainService.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.DomainService/DonateCoinDomainService.cs), [AddCoinRequest.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/ApiApi/Coin/AddCoinRequest.cs), [Constants.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Config/Constants.cs), [DailyTaskOptions.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Config/Options/DailyTaskOptions.cs), [coin-donation-policy.md](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/docs/coin-donation-policy.md), [BAC mirror action.md](https://github.com/bilibili-plugins/bilibili-api-collect/blob/master/docs/video/action.md).

### 5.5 Standalone like

BTP Daily likes through `coin/add`’s `select_like`. No separate video-like request appears in its inspected Daily flow.

1. If a standalone like feature is desired, submit the documented web action.
2. Treat duplicate-like response separately from successful new action.

**POST `https://api.bilibili.com/x/web-interface/archive/like`**

- Auth: Cookie `SESSDATA`, valid `buvid3`, `csrf=<bili_jct>`.
- Form: `aid` **or** `bvid`; `like=1` to like, `like=2` to unlike; `csrf`.
- Browser UA; exact required Referer/Origin **[UNCONFIRMED]**.
- No WBI/APP signature documented.
- Codes: `0`; `-101`; `-111`; `-400`; `-403` account abnormal; `10003` missing video; `65004` unlike failed; `65006` already liked.

Optional precheck: **GET `https://api.bilibili.com/x/web-interface/archive/has/like`**, account cookie, query `aid` or `bvid`; `$.data=0/1`; codes `0`, `-101`, `-400`. BAC warns this only detects **recent** likes and is not a reliable lifetime liked-state check.

Source: [BAC mirror `docs/video/action.md`](https://github.com/bilibili-plugins/bilibili-api-collect/blob/master/docs/video/action.md).

### 5.6 Configuration

Current class is **`DailyTaskOptions`**, section **`DailyTaskConfig`**. No separate `CoinConfig` or `DailyTaskConfig` class was present in the inspected source tree; coin settings reside in `DailyTaskOptions`.

| Key under `DailyTaskConfig` | Console/Web configured default | Meaning / class difference |
|---|---|---|
| `IsEnable` | `true` | Entire Daily workflow |
| `Cron` | `0 0 15 * * ?` | Scheduled daily at 15:00; class default null |
| `IsWatchVideo` | `true` | Complete missing daily watch; class-only default false |
| `IsShareVideo` | `true` | Complete missing daily share; class-only default false |
| `IsDonateCoinForArticle` | `false` | Try article donation first, fall back to videos when article service returns false |
| `NumberOfCoins` | `5` | Desired credited daily coin count; documented/UI range 0–5; runtime clamp caveat above |
| `NumberOfProtectedCoins` | `0` | Coin reserve; nonnegative integer intended |
| `SaveCoinsWhenLv6` | `false` | Legacy Lv6 donation stop |
| `CoinDonationStopLevel` | Implicit `0` | 0 disabled; 1–6 threshold; positive value overrides legacy switch; class has `[Range(0,6)]` |
| `SelectLike` | `true` | Coin-associated like; **class default false** |
| `SupportUpIds` | Empty | Comma-separated UIDs; empty/`-1` means none; malformed entries become an invalid sentinel and are skipped |
| `DevicePlatform` | `android` | `android` or `ios` for client operations; not used to replace web authentication in inspected Daily flow |

Legacy documentation entries:

- `DayOfReceiveVipPrivilege`: documented default 1; old day scheduling is no longer relied upon; separate `VipPrivilegeConfig:Cron`.
- `DayOfExchangeSilver2Coin`: documented default -1; old meanings -2 daily, -1 month end, 0 disabled, 1–31 selected date; current separate `Silver2CoinTaskConfig:Cron`.

Environment variables replace `:` with `__`; Console convention adds `Ray_`, e.g. `Ray_DailyTaskConfig__NumberOfCoins=5`.

Sources: [configuration.md](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/docs/configuration.md), [DailyTaskOptions.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Config/Options/DailyTaskOptions.cs), [BaseConfigOptions.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Config/Options/BaseConfigOptions.cs), [Console defaults](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Console/appsettings.json), [Web defaults](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Web/appsettings.json), [Daily configuration UI](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Web/Components/Pages/Configs/DailyJobConfig.razor).

## 6. Live sign-in and Silver2Coin

### 6.1 Live sign-in

1. BTP retains a live-sign API and `LiveDomainService.LiveSign`.
2. It submits the request and logs success or the returned message.
3. It is **not called by the inspected Daily workflow**.

**GET `https://api.live.bilibili.com/xlive/web-ucenter/v1/sign/DoSign`**

- Cookie `SESSDATA`; browser UA.
- BTP Referer `https://link.bilibili.com/`; Origin `https://link.bilibili.com`.
- No query/body/CSRF/WBI in BTP.
- BTP expects `code=0`, with `$.data.text` and `$.data.specialText`.
- **Availability disagreement:** BAC marks this activity **offline**, showing `code=1`, message `签到活动已下线，无法使用。`, `data=null`.
- A current “already signed today” code is **[UNCONFIRMED]**; do not hardcode a guessed value.

Sources: [ILiveApi.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Interfaces/ILiveApi.cs), [LiveDomainService.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.DomainService/LiveDomainService.cs), [BAC mirror `docs/live/user.md`](https://github.com/bilibili-plugins/bilibili-api-collect/blob/master/docs/live/user.md).

### 6.2 Silver2Coin

1. Separate `Silver2Coin` task, controlled by `Silver2CoinTaskConfig:IsEnable`.
2. Enrich cookies and validate account with `nav`.
3. Query live wallet status.
4. If `silver_2_coin_left<=0`, skip.
5. Otherwise make **one** exchange request.
6. Log returned coin/silver values.

| Endpoint | Request | Relevant response |
|---|---|---|
| **GET `https://api.live.bilibili.com/xlive/revenue/v1/wallet/getStatus`** | Account cookie; browser UA; Origin `https://link.bilibili.com`. BTP’s specific Referer is commented out. No query/CSRF/WBI. | `$.data.coin`, `gold`, `silver`, `silver_2_coin_left`, `coin_2_silver_left`. `code=0` success; BTP skips on nonzero or missing data. Endpoint-specific eligibility/error codes **[UNCONFIRMED]**. |
| **POST `https://api.live.bilibili.com/xlive/revenue/v1/wallet/silver2coin`** | Account cookie; browser UA; Origin `https://link.bilibili.com`; Form: `csrf=<bili_jct>`, `csrf_token=<same>`, `visit_id=<generated client ID>`. No WBI. | `$.data.coin`, `gold`, `silver`, `tid`; `code=0` success. BTP’s external-test source expects `data.coin==1` on success; that is a test expectation, not an exercised result here. Specific insufficient-silver/daily-limit codes **[UNCONFIRMED]**. |

**Confirmed limits/behavior:** obey the server’s `silver_2_coin_left`; BTP attempts one exchange per task invocation and does not precheck a hardcoded silver cost. Exact current exchange rate, fixed daily exchange count, and eligibility restrictions were **[UNCONFIRMED]** in the inspected allowed-source material. In particular, do not implement a guessed 700-silver rate or fixed daily limit from this report.

`visit_id` is generated once for the request class: a leading digit 1–9, ten generated lowercase characters, then `0`.

Default separate schedule: `Silver2CoinTaskConfig:Cron="0 0 8 * * ?"`, `IsEnable=true`.

Obsolete API declarations remain in BTP, but the current task does **not** use them:

- GET `https://api.live.bilibili.com/pay/v1/Exchange/getStatus`
- GET `https://api.live.bilibili.com/pay/v1/Exchange/silver2coin`

Sources: [ILiveApi.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Interfaces/ILiveApi.cs), [LiveDomainService.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.DomainService/LiveDomainService.cs), [Silver2CoinTaskAppService.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Application/Silver2CoinTaskAppService.cs), [Silver2CoinRequest.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/LiveApi/Silver2CoinRequest.cs), [wallet response](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/LiveApi/LiveWalletStatusResponse.cs), [exchange response](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliBiliAgent/Dtos/LiveApi/Silver2CoinResponse.cs), [external-test source](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/test/Ray.BiliBiliTool.Agent.FunctionalTests/LiveApiTests.cs).

## 7. Common request conventions and risk control

### 7.1 Headers and serialization

BTP adds missing common headers:

```text
Accept: application/json, text/plain, */*
Accept-Language: zh-CN,zh;q=0.9,en;q=0.8,en-GB;q=0.7,en-US;q=0.6
Sec-Fetch-Dest: empty
Sec-Fetch-Mode: cors
Sec-Fetch-Site: same-site
Connection: keep-alive
```

Endpoint attributes override/add Host, Referer, Origin and special headers described above. Form and query parameter names are **case-sensitive**: BTP normalizes the initial character to lowercase, preserving underscores, e.g. `Played_time` → `played_time`. Sending CLR-style `Aid`/`Csrf` caused `-400` according to source comments.

**Console/Web configured browser UA:**

```text
Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36 Edg/124.0.0.0
```

**Configured APP UA:**

```text
Mozilla/5.0 (Linux; Android 12; SM-S9080 Build/V417IR; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/91.0.4472.114 Mobile Safari/537.36 os/android model/SM-S9080 build/7760700 osVer/12 sdkInt/32 network/2 BiliApp/7760700 mobi_app/android channel/bili innerVer/7760710 c_locale/zh_CN s_locale/zh_CN disable_rcmd/0 7.76.0 os/android model/SM-S9080 mobi_app/android build/7760700 channel/bili innerVer/7760710 osVer/12 network/2
```

`SecurityOptions.UserAgent` class-only fallback instead uses an older Mac/Chrome87/Edge87 UA. Gaia uses its distinct Edge151 UA from §4. Bruno examples use still another browser/app version; do not confuse those with active application defaults.

### 7.2 Timing configuration

| Configuration | Documentation | Current source/default behavior |
|---|---|---|
| `Security:IsSkipDailyTask` | Default false | Console task-execution gate |
| `Security:RandomSleepMaxMin` | Console default 0 minutes | Class default **10**; Console overrides **0**; Web file omits it. Login/Test skip startup random sleep per docs |
| `Security:IntervalSecondsBetweenRequestApi` | Docs say **20 seconds** | Class default **3**; Console config **10**; Web config omits it and inherits **3** |
| `Security:IntervalMethodTypes` | `GET,POST` | Same class/Console default; empty disables matching methods |
| `Security:UserAgent`, `UserAgentApp` | Request UA controls | Values above |
| `Security:WebProxy` | Empty | Optional proxy |

For a matching request and positive interval setting `N`, BTP delays a random integer number of seconds in **`floor(N/2)..N`, inclusive**, before sending. Thus Console default is **5–10 seconds**, not a guaranteed minimum of 10 or 20. This is a per-request delay, not a global cross-client rate limiter.

Special paths use fixed delays: lottery join 3 seconds; live trace `/E` and `/X` 1 second. These do not change the Daily endpoint behavior above.

### 7.3 Errors and retries

| Code | Documented meaning / BTP treatment |
|---|---|
| `-101` | Unauthenticated; cookie guard marks invalid |
| `-111` | CSRF validation failure |
| `-352` | Risk validation failure; BAC mentions invalid UA or WBI parameters |
| `-412` | Request intercepted, documented as client-IP risk control |
| `-403` | Generic access denied/account abnormal depending on endpoint; BTP share explicitly stops without retry/device rotation |
| `-106` | Not a formal member or probationary account |
| `-113` / `-115` | Real-name verification required |
| `-650` | Account level too low |
| `-799` | Requests too frequent |

These public codes are **not a guarantee every endpoint returns every code**.

BTP has no generic JSON-body `-352`/`-412` retry strategy in the inspected HTTP policy. Risk handling is endpoint-specific: signed video-detail route, preserved/enrolled device identity, skip share on `-403`, abort unexpected coin errors, and treat failed validity checks as unconfirmed rather than expired.

Transport policy constants: timeout **30 seconds**, one retry, **2-second** backoff. Read-only policy uses Polly transient HTTP errors plus HTTP 404. **Source comment/implementation conflict:** `MutatingPolicy()` says it retries only network errors, but its actual code uses `HandleTransientHttpError()`, which also covers HTTP 5xx and 408; it therefore does not implement the narrower behavior claimed by its comment. Neither policy interprets Bilibili JSON business codes.

Sources: [common-header handler](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/HttpClientDelegatingHandlers/BiliBiliCommonHeadersDelegatingHandler.cs), [form-key normalization](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/HttpClientDelegatingHandlers/FormUrlEncodedKeyNormalizingDelegatingHandler.cs), [interval handler](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/HttpClientDelegatingHandlers/IntervalDelegatingHandler.cs), [SecurityOptions.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Config/Options/SecurityOptions.cs), [BiliResiliencePolicies.cs](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Agent/BiliResiliencePolicies.cs), [configuration.md](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/docs/configuration.md), [Console defaults](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Console/appsettings.json), [Web defaults](https://github.com/RayWangQvQ/BiliBiliToolPro/blob/main/src/Ray.BiliBiliTool.Web/appsettings.json), [BAC mirror public error codes](https://github.com/bilibili-plugins/bilibili-api-collect/blob/master/docs/misc/errcode.md).
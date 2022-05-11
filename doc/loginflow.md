# Juju CLI login flow

This is documenting what happens when a user does `juju login -u admin` at a
point when no user is logged in.  It has been done by observing what messages
are exchanged between client and controller in the websocket connection and what
http requests the client makes to the controller.

In the following the controller host is `CONTROLLER_HOST` and the admin password
is `PASSWORD`.

Read this document for an easy intro to macaroons:
https://github.com/rescrv/libmacaroons/blob/master/README.  For a more
theoretical approach you can read this:
https://static.googleusercontent.com/media/research.google.com/en//pubs/archive/41892.pdf.

## Overview

Login with just username and no creds:
- triggers macaroon discharge workflow to ultimately, all going well, get a
  macaroon with a TTL of 24 hours and username caveat, stored in client cookie
  jar
- first step is for client to prompt for password and redirect to trusted 3rd
  party identity provider to validate identity
- results in short lived "login macaroon" which proves the user has logged in
- controller then mints the "real" macaroon to return to the client with
  declared username caveat and op set to login
- client logs in again with this newly minted macaroon and auth passes and login succeeds

## 1. Get websocket connection
Open a websocket connection to `wss://CONTROLLER_HOST:17070/api`

## 2. In websocket: first Login request (fails)

The first thing that happens in the websocket is an attempt to call the
`Admin.Login` endpoint. 

Request:
```json
{
    "request-id": 1,
    "type": "Admin",
    "version": 3,
    "request": "Login",
    "params": {
        "auth-tag": "user-admin",
        "credentials": "",
        "nonce": "",
        "macaroons": null,
        "bakery-version": 3,
        "cli-args": "[...]",
        "user-data": "",
        "client-version": "2.9.26"
    }
}
```

Because there are no credentials or macaroons in the request, the response
contains a macaroon with a third party caveat that needs to be discharged to
authenticate the user. This macaroon is made in
`apiserver/authentication/user.go`, `UserAuthenticator.authenticateMacaroons()`.

From looking at that code, The condition of the 3rd party caveat is
`need-declared username admin`

> Q: I guess it's encoded in the "v64" value below?

Response:
```json
{
    "request-id": 1,
    "response": {
        "discharge-required": {
            "c": [
                {
                    "i": "time-before 2022-03-10T09:26:13.554951585Z"
                },
                {
                    "i64": "AwA",
                    "v64": "84lQ1w2X1y5_pzw6J43_UgGHkExE_T97jn95tTHEZAgaJW3IgRuNI7Yk0qpLM11_kPQ497PxW9yiv6sShpvgUYg7vgPRHi0r",
                    "l": "https://CONTROLLER_HOST:17070/auth"
                }
            ],
            "l": "juju model 10c91043-b22b-4e62-8f44-30132106b057",
            "i64": "AwoQOJOvmtzn4H9e3RXl7OzfUhIgOTQzM2Q1MmFlNWY3ZjdmN2U2NzdhZjU0YzllMTcwYTkaDgoFbG9naW4SBWxvZ2lu",
            "s64": "jAUuaROGZqMCjIoD-BSow9GCMlxELWNjYGArgqeLK0Q"
        },
        "bakery-discharge-required": {
            "m": {
                "c": [
                    {
                        "i": "time-before 2022-03-10T09:26:13.554951585Z"
                    },
                    {
                        "i64": "AwA",
                        "v64": "84lQ1w2X1y5_pzw6J43_UgGHkExE_T97jn95tTHEZAgaJW3IgRuNI7Yk0qpLM11_kPQ497PxW9yiv6sShpvgUYg7vgPRHi0r",
                        "l": "https://CONTROLLER_HOST:17070/auth"
                    }
                ],
                "l": "juju model 10c91043-b22b-4e62-8f44-30132106b057",
                "i64": "AwoQOJOvmtzn4H9e3RXl7OzfUhIgOTQzM2Q1MmFlNWY3ZjdmN2U2NzdhZjU0YzllMTcwYTkaDgoFbG9naW4SBWxvZ2lu",
                "s64": "jAUuaROGZqMCjIoD-BSow9GCMlxELWNjYGArgqeLK0Q"
            },
            "v": 3,
            "cdata": {
                "AwA": "A6Ve5aR-tWB-Q0M7ziC2TW_oC9RxW0PwIjQI_eVUhslCKaXcdTmTr5rc5-B_Xt0V5ezs31IttCIUNp99pEQKvgrQDDBRGbJ1SXRGp9WOt8DJpr9dZRfIp_nIG4S0LSzrLnF0TiEy4jZvFcXucApDF7GgPULqnwVX7X_l630n-ZCsTcdZP1W2vnJKsUc_po0jYemzyUBDVckiJz1hDgo"
            },
            "ns": "std:"
        },
        "discharge-required-error": "invalid login macaroon"
    }
}
```

## 3. Discharging the third party caveat

The client must now try to discharge the caveat, which will prove that the user
is who they claim to be.  This is done by contacting an auth server (in this
case implemented on the juju controller) as follows.

This flow is implemented in
`github.com/go-macaroon-bakery/macaroon-bakery/httpbakery`, in the
`Client.AcquireDischarge` method.

> Q: Is this the macaroon/Candid flow?

### 3.1. HTTP: First attempt to discharge macaroon (fails)


`POST https://CONTROLLER_HOST:17070/auth/discharge`

Query params:
```
caveat64=A6Ve5aR-tWB-Q0M7ziC2TW_oC9RxW0PwIjQI_eVUhslCKaXcdTmTr5rc5-B_Xt0V5ezs31IttCIUNp99pEQKvgrQDDBRGbJ1SXRGp9WOt8DJpr9dZRfIp_nIG4S0LSzrLnF0TiEy4jZvFcXucApDF7GgPULqnwVX7X_l630n-ZCsTcdZP1W2vnJKsUc_po0jYemzyUBDVckiJz1hDgo

id64=AwA
```

> Q: even though it is a POST request, query params are used instead of form
> data in the body.  Why is that?

Response:
```json
{
    "Code": "interaction required",
    "Message": "cannot discharge: interaction required",
    "Info": {
        "InteractionMethods": {
            "juju_userpass": {
                "url": "/auth/form"
            }
        },
        "VisitURL": "/auth/login?waitid=fef72f1d163d4f6d2eb01a49",
        "WaitURL": "/auth/wait?waitid=fef72f1d163d4f6d2eb01a49"
    }
}
```

### 3.2. HTTP: Obtain a token to discharge macaroon

The client chooses the "juju_userpass" interaction method, asks the user for
their password on CLI and then verifies the credentials.

`POST https://CONTROLLER_HOST:17070/auth/form`

Body:
```json
{
    "form": {
        "password": "PASSWORD",
        "user": "admin"
    }
}
```

Response:
```json
{
    "token": {
        "kind": "juju_userpass",
        "value": "MDVhZjU1NDM3NjYxZGE5YTJkMGMxMDky"
    }
}
```

### 3.3. HTTP: Second attempt to discharge macaroon (succeeds)

Now that the username/password has been verified, we have everything we need to
get the discharge macaroon.

`POST https://CONTROLLER_HOST:17070/auth/discharge`

Query params:
```
caveat64=A6Ve5aR-tWB-Q0M7ziC2TW_oC9RxW0PwIjQI_eVUhslCKaXcdTmTr5rc5-B_Xt0V5ezs31IttCIUNp99pEQKvgrQDDBRGbJ1SXRGp9WOt8DJpr9dZRfIp_nIG4S0LSzrLnF0TiEy4jZvFcXucApDF7GgPULqnwVX7X_l630n-ZCsTcdZP1W2vnJKsUc_po0jYemzyUBDVckiJz1hDgo

id64=AwA

token=05af55437661da9a2d0c1092

token-kind=juju_userpass
```

Note that the value of `token` in the query params is the base64 decoded value
of the value of the "value" field in the previous step's json response.

Response:
```json
{
    "Macaroon": {
        "m": {
            "c": [
                {
                    "i": "declared username admin"
                },
                {
                    "i": "time-before 2022-03-10T09:26:18.497625359Z"
                }
            ],
            "i64": "AwA",
            "s64": "Jcdpeh0Nmx8f85wWyVF3w7gB1RzYNuWoNyLiCGRe5Q0"
        },
        "v": 3,
        "ns": "std:"
    }
}
```

Now we have a macaroon that "discharges" the third party caveat.  It just needs
to be combined with the original macaroon to make a macaroon slice that allows
authentication.

## 4. Websocket: Second call to Login (succeeds)

Request:
```json
{
    "request-id": 2,
    "type": "Admin",
    "version": 3,
    "request": "Login",
    "params": {
        "auth-tag": "user-admin",
        "credentials": "",
        "nonce": "",
        "macaroons": [
            [
                {
                    "c": [
                        {
                            "i": "time-before 2022-03-10T09:26:13.554951585Z"
                        },
                        {
                            "i64": "AwA",
                            "v64": "84lQ1w2X1y5_pzw6J43_UgGHkExE_T97jn95tTHEZAgaJW3IgRuNI7Yk0qpLM11_kPQ497PxW9yiv6sShpvgUYg7vgPRHi0r",
                            "l": "https://CONTROLLER_HOST:17070/auth"
                        }
                    ],
                    "l": "juju model 10c91043-b22b-4e62-8f44-30132106b057",
                    "i64": "AwoQOJOvmtzn4H9e3RXl7OzfUhIgOTQzM2Q1MmFlNWY3ZjdmN2U2NzdhZjU0YzllMTcwYTkaDgoFbG9naW4SBWxvZ2lu",
                    "s64": "jAUuaROGZqMCjIoD-BSow9GCMlxELWNjYGArgqeLK0Q"
                },
                {
                    "c": [
                        {
                            "i": "declared username admin"
                        },
                        {
                            "i": "time-before 2022-03-10T09:26:18.497625359Z"
                        }
                    ],
                    "i64": "AwA",
                    "s64": "AZiBA_Qhxevs54KGBgGQz59jDzMIqtCtg31Rz-v0Vcs"
                }
            ]
        ],
        "bakery-version": 3,
        "cli-args": "[...]",
        "user-data": "",
        "client-version": "2.9.26"
    }
}
```

Success!

Response:
```json
{
    "request-id": 2,
    "response": {
        "servers": [
            [
                {
                    "value": "CONTROLLER_HOST",
                    "type": "ipv4",
                    "scope": "local-cloud",
                    "port": 17070
                },
                {
                    "value": "127.0.0.1",
                    "type": "ipv4",
                    "scope": "local-machine",
                    "port": 17070
                },
                {
                    "value": "::1",
                    "type": "ipv6",
                    "scope": "local-machine",
                    "port": 17070
                }
            ]
        ],
        "controller-tag": "controller-6d7d0374-6ef2-422c-8216-1354eacc11d4",
        "user-info": {
            "display-name": "",
            "identity": "user-admin",
            "controller-access": "superuser",
            "model-access": ""
        },
        "facades": [...],
        "server-version": "2.9.25.1"
    }
}
```

Done!
/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and/or data
** (collectively the "Software"), free of charge and under any and all copyright
** rights in the Software, and any and all patent rights owned or freely
** licensable by each licensor hereunder covering either (i) the unmodified
** Software as contributed to or provided by such licensor, or (ii) the Larger
** Works (as defined below), to deal in both
**
** (a) the Software, and
** (b) any piece of software and/or hardware listed in the lrgrwrks.txt file if
** one is included with the Software (each a "Larger Work" to which the Software
** is contributed by such licensors),
**
** without restriction, including without limitation the rights to copy, create
** derivative works of, display, perform, and distribute the Software and make,
** use, sell, offer for sale, import, export, have made, and have sold the
** Software and the Larger Work(s), and to sublicense the foregoing rights on
** either these or other terms.
**
** This license is subject to the following condition:
** The above copyright notice and either this complete permission notice or at
** a minimum a reference to the UPL must be included in all copies or
** substantial portions of the Software.
**
** THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
** IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
** FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
** AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
** LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
** OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
** SOFTWARE.
 */

package transport

// Test file for pkcs8_encrypted_private_key.go
//
// Strategy:
//   - parsePKCS8EncryptedPrivateKey: tested end-to-end using real PEM constants
//     generated once via OpenSSL as per the below commands.
//   - All helper functions (stripPKCS7Padding, hashForPBKDF2PRF, keyLengthForOID,
//     decryptCBC) are tested directly with constructed inputs.
//
// OpenSSL commands used to generate the PEM constants (run once, output pasted below):
//
//   openssl genrsa 2048 > base.pem
//   openssl pkcs8 -topk8 -in base.pem -v2 aes-256-cbc    -passout pass:samplekey
//   openssl pkcs8 -topk8 -in base.pem -v2 aes-192-cbc    -passout pass:samplekey
//   openssl pkcs8 -topk8 -in base.pem -v2 aes-128-cbc    -passout pass:samplekey
//   openssl pkcs8 -topk8 -in base.pem -v2 des3            -passout pass:samplekey
//   openssl pkcs8 -topk8 -in base.pem -v1 PBE-SHA1-3DES   -passout pass:samplekey

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"crypto/rsa"
	"encoding/asn1"
	"encoding/pem"
	"strings"
	"testing"
)

const testKey = "samplekey"

// One RSA-2048 key encrypted four ways — generated once via OpenSSL.
const pemAES256 = `-----BEGIN ENCRYPTED PRIVATE KEY-----
MIIFNTBfBgkqhkiG9w0BBQ0wUjAxBgkqhkiG9w0BBQwwJAQQ6SqQj94fJw5WqBGY
4U78rAICCAAwDAYIKoZIhvcNAgkFADAdBglghkgBZQMEASoEEJIMx2yqr9tJ/U9G
F7nFNQwEggTQ4KuJLxJIw11a7PqFm9mSUdWFOMDsPqT17H8WuIkTnBw/jw53mqwh
7tdZGX4ojVpKDvJAZ8DNqTOdo34A9FqJ1imqXTsQvHiGJNK5KcB1PiriEA+ur3XN
35gKWmSCplXw20puWcJZeEiNzJhWPNrVA5l5ouB7jOui9Hqzv15TnWdG9wJ0hikM
4U/T/niL3f8mdK5bEbk3x+1jYHBFYit6VZCfoul0xRaa8UMG+9fZQcjw3XKGI9tO
cF11Cb2D1SFFbKvbEzAE4a+IywVI1py/1XczdACQGYO8EeEihsWWQZn2bDv7OScm
xfriVWtIgLa3r1wlsOxnVaIEk03QgwBu/O3kGzxXOKZxC86wgHTcm9qs+kin64PS
c4jfMnGfo1FLgmxMB2n3pnsJA4OQ4R+dr/AR0/mMMlG9+xWEa+omIXOOOIuvyj1s
OucZ+bSeutaS0C+NKKCpADtqFMIQwzDtN1B/+EsB7Aai7rCofrhhTw86ysDganMd
onlI6JvO7+1NsfN1A4kaHGt/IRplDIDLKozF+YlUTnz8Gbtuz4IR8HQYAn9P2CCG
l60Rh/cHAxkSNlBl/hUZY23UyG/BR1aFF8o1/dgKdYSMKslBxAz00dUhnuhNN1T8
L4AJ9rBf5aNkwtqJ48ab9nZAUSef617Qe8TxpDcQXGzzRueNw5R01Tr6N+wzrMwb
94QSyyLshUUd4sxsrKSxIxxABjQYiYSuqSnOazVAce3HkrvVJs2TAZOd6PWZn3fY
VaaLnKiH3ezDD+nc5viEWfHsELfV56UG03eMLxLwex0E/d17VpW2NRFpShOH2Wau
OdxzYqMes3ti410ObcZXZ1Sy6HsI/VjZq8SSV+jyzIfBes20A/NiETAg2QthS0p2
xxT/xQHWvNPap+18VZAWuUClKQ6hqrcZ893eigIstAbzeLHGJuFVzPOYi+O9jEsa
SqMnOLO1nL3dMkMLsogNwUy4sFtZ9ci8K8MjF6MAyZlU3FdlyrRdwPfh1U/q5jAX
omj5seB79C05ASsjdT64zZ5lztORdbZrR482CAGHGtrhv3n/NWjTG85Nmu+55rXK
9gnTDLAGJ9s4UqlKrV+Hp1+a2GZHfG/pJTsENj0xjEQ5/X7Oh3/evG5jUDFflEMf
BuFiy/z56e/h6UWEpjhcaJTKYWxvM75hh4t3eQjJFZ1eOQQnDy/JsFTO2klJ+XjU
jIPXwrozSpmO+bVyfASqf1TueQ8JL011ZvYyGf3h+OPwCIKjrR3tK2XDAzOo25zr
hSf2OG2L4F8dMyDkqtviEcnOPGVBsgwSJvw0UVdsxNl3lTxUy2zOPxxG/gFHPkCh
0516HN3YHpk26ZpJ08RnCrNcx4RMQYio81CmECjWT51vk+NNsqALxNboq8YXYF8a
7pA67wu8+i4IAyUdClAc4972y5+/B9ImNVsYocXZSRaCSUu0nLhyelHSV/D08oM0
o1S0RH7Ue7YcVp3oIA844WSqwvjrAdNthxWY7eSTiRMLCNjYeFzqe+AksvDgW6G3
C6rw3sVdZjnezQDr7q/m5OSxLH33xf9KV0mU9UTmwpNimstaTfFkj49lXK24a1vt
zcvk6nNhp7NQfhkWmFEa7gDj8GvN9ddbtleIWOfGajened0NVL5s9lw=
-----END ENCRYPTED PRIVATE KEY-----`

const pemAES192 = `-----BEGIN ENCRYPTED PRIVATE KEY-----
MIIFNTBfBgkqhkiG9w0BBQ0wUjAxBgkqhkiG9w0BBQwwJAQQU0sRzL6bcaoRYgl6
PQPBcQICCAAwDAYIKoZIhvcNAgkFADAdBglghkgBZQMEARYEEC6gXVMCF9cfzPme
cMHO1ekEggTQiKEbDfvYVMMZeMpOW8GCZDontm0kgZLHN26t6uxAPui9SlsZHBIe
0KF9EB7Wl9Cjzm84zb+cve166B+uO9mPIP+du2E0pz+Ojny+p6zK/o3vNzw8Fll0
Ov9QpkbNamcUyo05aN7xn8l73StdivwbJKx3Z1Kn61/7UHY+u+hwJFo9lQJSM7gl
zSD9GfNyodCq0XWWjrLwexAiHZp3qPFYn7e1q32ld5A5EZjOCIhbtQBpZpRDtZU5
zNWaGiU46AkE3Aq1qHPvlFy3wqe2oRs7zZ2gE0OZE7lYzm76IjMIAPtEDrTq3K9b
DWJkMJkrYg3692l20ooHGNeFrFmYRI6xB/HS6oal/jkGas1eGqxd/rJAyx5zTit1
6jbkVlyUQMWvOfJmL35gW3fp44fUgnXcRIdMOIUOe2vaOKDgEGus0F/qNfesNcwQ
MsGYkFq+XPen4v4wRqsPHwY73mZ3XnScGfnUY8GF+nCVrW02zcOCNudJS5ExVcTr
UkJZUgQbxbKp/XA0BSGVOSj0M0AIGBCE57KWtEwYdRdBazlociImUd/bDjVWPP3T
997TgMDQmWKWHDX84hK0b19OxXsfBpdfDWLhwJkP/chFubG7jdOOPGmQfyLv1jnB
rc04aUgrVg30w8T2BZbyr5iORRCZlksAM+1jlNwqBIE3qGaH2LA18ApU63UFq50O
ZBN7a3EcqhPW+CovkHxS0xQzEcRNuwj9nWZzkGnxwCYvIKBKZx6XdQgauCR52dhj
KKJJFZXYpeS/KLmDwBkfS4zsRN1S+7WT4FBvnHbz6VdMsRoKb6O+ofGBCCbYHZNW
c8JjYStXr2smHzgDNW1YJYG421mu8O1hba91JYMJfEkKn+XkCxr29qKrTchyQJHw
jeGXjz9ZewLXxF7IV5vqmLIMzu+xb2SbLOXkUX8+mWLUek2RXumfRjnq0ou3Xhlg
x1cc/U+HG5JYWoZGJSSavyCr5RRaDTniBHvVUKKQtMv3144BTfeZ39xlpHvVFU0O
UMcy5J16AzB5otnmMOFiKIAggpUqjNajm9dxbnX0rHx8xikUyxKNf3LT+jbDdfTp
e2gQCYpSgnvVuwWb17PShhUXp4Z/idkEYhTUILIscli0AqjMf0K6WkMsVjNpIIx3
VQ5fPoSVwA/2WKltvv9yEhbBoydwK4c5LoYrLJtz/tSThnLPm6GwxZsahcFlusph
DZAlX0lclNhsTtsUB+4zylYwahS93KFexj0LqCLEHoGUu9JiukhxUuzmh+uNEIRK
mhZN4NPwRVau8Qvd5Zh70PD0PkQwyf0pBvjykQKEBqoKErh4Q/c6zbkuVnLizK9y
Hc4P7lESSqPAworsJf1Kpyq7gBMHxBGe4HYHlEVTndn1CYpqdIHPB1pTd54fHz+u
KC3PD4TxiFTH3wZlEj5xfBuw2EMXq6EnjZ1+4To2ihm9eW51AknqfdY7RqjQG9N2
mPJuMkbA6/gViCGuK3nyPgSeA6DtLz1IPvcOaW0AcQtFc7a6YgBjU4wG3PkCaL+x
E/0+uE5tgeB225lBZY5FDH0de1vA6KWAb85xb7v5fXarS7NwUQMey7MdSWRiINdo
bmp+3vtpNWof4B7h0SkrM8uFLfbAZJS3diJMcOFgqs4Hv09TgmXtzQw=
-----END ENCRYPTED PRIVATE KEY-----`

const pemAES128 = `-----BEGIN ENCRYPTED PRIVATE KEY-----
MIIFNTBfBgkqhkiG9w0BBQ0wUjAxBgkqhkiG9w0BBQwwJAQQFPc4qGUyKcgIQCx1
uTb2VQICCAAwDAYIKoZIhvcNAgkFADAdBglghkgBZQMEAQIEECJ4ZCh1RxZ9aEov
713Hge0EggTQ+9mr3XKRETxKJdEzd6wpSIlRCQKgC7DDZUHejSaeKchN08HPM7wK
FRPdqjC/ILcLslN13Cw1dHaUAQ2vuNJUznC72v6E4RVkdLMldQemstRbdARfFyfp
F5uOiyfMDe33/yB81dKyTULSPhjeXq+4poNUmj5/na4uuDx7bnPdgMRbwOSTUSSo
06Px8qCK6ZKVTRETPlODvC6TVODmLlxdQ34j3hs9VCwPsq/y0oyQX7zLGNkFVun2
7Z1/NrTs1+fxi8Ppe0Ph/Xo6PEJJWHRnCdRGgRIWzNyx7CowxjPcKoeKqleVgKSZ
vGwJCJDdwH0zLW20EM9Q3a/Yt2N5Z6Ez/g7PDEwV9zOjbsx292dYcyxqic3olA9f
Cp7FcAVYXT3sSdMr/AJzWNAF20snLsjQl2CdO+mkxjvjkSS7c8qugFAdUmu5tiqU
FP5bMzyNh6PxxI14xoxftcYtgwErylTKPoURqgHlQ4khcsI9L5G6FfrCkbVGVXcA
amIMDzCcG0Bbizdu1NJkIdic1gzXuoPuYAIt0KBzws14vAuADSNcVZTBJxIWWS8i
HFOGMHD+rA5Od3e6CfKxzYUdPOFQGKVkD1gubPldWYC94e/PXp3lbuui6IqSfFez
xHJA/W9xPJMi28HgbyhhkkDacHcQbLoY7cV1AZBxsn0nLwMrng/EjTeYw/pUSyAk
KVUTj8ELUosGsdM5uOZ3sKeXhG2xEBWT0HGcIQkaNuQzMWomw2QDHDWRiMclKM57
hO9zUzDaqvWyRjR/H4/Oky2KOZ2mZtnOSsc0qA/u9XPCB4x7JIt87F40SQkPOsFS
GxEitdJKR6XH6QxWDGe0me9NUyRvbIdyzmQwCZ2APi3OBs4HLemNOYykfCDUK805
6VoPYiufFVnkaXriY808LI9uN7qPO1a2e4KCKPMuXJIM2z51atK9FBRIlPQ44YPl
xEGTC2+kgzTLZkt7HjebpDJ2tTX6hYrtBN70Xi/eM+2JDZL/laOzO+tcVo6YwTjy
xqd1a8H3dlgccEgC4NRVYIvqgdRk6fvh4pL4IyJ208uTRRo486gptVv4yLZWRDlv
KH743aj4wJbIIWqaq65Dk4j5YNKoBF5rn6kjoH+4KVd1zfGr1E030XaGwKJ3TVWN
43veF1CV1LLvTgcde+ckyC8pgngcAVpxfJFgxTiqFiFhxXxjoqYKyVxiE1Oe2gM4
5ky/fxKmYc++Cw0WxDpSfVmkT942Rc2ZzhUEnQ3MqPyJ6LkedC0B9+hOy3hzuaMk
8r/VOfatbTbvnHBu9w1XItXVXWsH5bWqVcl6C5zqaSzq7Vy26jgZEDXRkdtcwV+m
xRJacXaXs85E842mpqXULkPXc0LXAKU1ukGb6W6t9pjwjvaReXH5l+N+7EeeE70p
UxfOJMndRkhiRtKftc/b31hLMnAx+bXYNIe08bqU0F3vLozsudtJu5n6QVA+QQgC
XpPB9Utu3fWwhHyYFlaCf82tftECH4Djm0D/12vgZbSWRzUHU/KU+MAgLT4YZG+q
XSfTUIwehrYqM5sh7zE4qnTSeGQqZ1UR7OjLQ1mV5roB5kbdLmD1YdhMLKK3Ztbj
cyFWGP1rWh6zeC2enrfYNn2PJ9cwNGD97NLhZ1gTC33Wub3IfRHRu8s=
-----END ENCRYPTED PRIVATE KEY-----`

const pemDES3 = `-----BEGIN ENCRYPTED PRIVATE KEY-----
MIIFJDBWBgkqhkiG9w0BBQ0wSTAxBgkqhkiG9w0BBQwwJAQQ8j//lRGepCHdlCr0
Af4vogICCAAwDAYIKoZIhvcNAgkFADAUBggqhkiG9w0DBwQI8vusaN+sazAEggTI
BUmYrP6Tu63jWEjG4ukmsrtSf6cDG/Q+BbZvPez9hsyEtvtivFAp8WsMQaAXxyyJ
p7IvS6Efr+iuASUAvQL/kc77ya9hDOOswwxDVNfYqpK2D8Utfqp1Lwxs55rs9lnd
mA1lGFBMzlAyN6VVKF3Je+kMEM3oFp4gTjCsqlkI+BAKlDQ5qVSVSgcp5h7BwRRc
toaUHE6M6JOGbJGXhkkWfFQgvAkKt/w+llDZdH3mrPhCzJlxqJmyr2zJZ7Esk1ic
EkOXEMsqoJ1lPBkN/5SxFznCZaF7/xG6+IVGUTaSK/WsYYjkpS9tvNM2PkDixX6w
H/ig9KUF4/kSmigJKwUqgtrqYbtRWvY6s2jeHE2RWR33SumfJgJYjrYIlZDKcKrL
M7Um4wqnN8ni4MA/HKLxmieobHulK4jCDwbnjgyq4jx3+EdcNDAFL9wwpcT5QJE8
rG9VIqxSHnrJeesgbTV/yuE56Gjj98q1zZsBya1eTYM4O6gGdWGAD6ly9k9Po9oi
Xrazp3mzgCMy7JK26bgSIjDWecCJGjl+3q9bQ6fX6n9fHvReEn8effZ/VZBeOpul
w8+7rjMbN9szZi/4AgE5BzOifdKkW8tekOFU71eD6isB5x8oH+uiFKUtln2hPsBf
ulb+ocefZ6i/j8vnNvMZ758xhcVJdqB150zxunZEAesC1v67z/0vMtrUW+aTYMVc
tLOmCUWz0dOHAZOs1pzdxcZtCXegt2Wz6HkQLhlD14/7D5h4XWOwKYQ6K3iZyU2h
SzPbj/hhDp1zaEuZ9KuobAjx1t/JQmRNeTiLXiophyvzqaXa3LJ5tm+V+bNpjWiH
pFDC+/6U7qnyHdxlTXevuxY8xMlogBxj6ZNMqqGR05Gk0WlG6HjVXDMzi8lL5kHg
NJAdbb26vtitZYl5ukqSRAo3O+WafDkeEuB9awLgrb1QWnX4NGQkl4IaWBmmfHjT
tro42xqKcjLMS8wvMI2WsJ0HKFR11NdRgsL0vseITpCLQESSHwCsxzKT6oFPZTjB
4N/R3bCu3zWTJ9vy7i+RLQEuEZ9n9th3TlpToukivWReSgT+7gDndqjEb/ITGqhD
LdOQWvzL0l6wfAak7Gv0AMHgBllJm9Ye4ZtD7U8EJWohxGe67F3SPJlf3a6hDNBN
1LGnhovpHhCRbgGZMj3uyuVpBE0YR2zStcbNrWKev8/Nfpb8fhL+tiCIEdfg9+/X
SY29W+Y91XpqRztNkljHaKJTq33uqZEQyhYUoXH4xO3Bb4JCNRf9YwFProxKV+Uk
M7NHU1KIoaFL9430zAP531KeX5CjD4lCJANRHThZ2A/6dJVCt7fcOSodbndnE2KG
YEDveWVUO0yzjpX4CPir8FQ7pNuT5rdXY8U3H4YM6fVWsk6dAdg5/UUjrb+QfA9W
/bDNLcdHcDla9KHf1ka6uCvOg66Eu8vzhTuHv/UwgMIoHYggLoK4uecX0jHg26zO
kgKKUvWgTINwCaO7Fj56uAxTyynpdJp62fZoKQV+yXOCdNgw4eMTeaWCQTYfomjy
VKYrPuluGY7Bt4le6JUvi/8pm/Oao2di6uN2L3I+NRUTMHg+uCdPP5syG2oROYUT
0r4DRlwd7c+azovYz8f2g/6i1g/WOb/D
-----END ENCRYPTED PRIVATE KEY-----`

// pemPBES1 is an RSA-2048 key encrypted with the PBES1 scheme PBE-SHA1-3DES.
// Its outer algorithm OID is pbeWithSHA1And3-KeyTripleDES-CBC, which is not
// oidPBES2, so it exercises the top-level OID rejection branch in the parser.
// Generated via: openssl pkcs8 -topk8 -in base.pem -v1 PBE-SHA1-3DES -passout pass:samplekey
const pemPBES1 = `-----BEGIN ENCRYPTED PRIVATE KEY-----
MIIE6jAcBgoqhkiG9w0BDAEDMA4ECD1MXPCJ6pY7AgIIAASCBMhNm/dX3sRraq5x
o4pvh3pIoh2RMR4dAb3io+gHSVAq4aBbnTn5jhveh1bLWDVLdJFFj0f5CjnwX3Lx
ia7/uRdGWRyIfFwX7oMuRnshmpX71Lj1LO7GEAETmswOZ0v8CiWnAsBkXRvunYZo
vT44vtll2CUvyKTNnRIXg5cVGBjgNuWSFrB5StYuqNOZ7CQGoZuX9zdWnWMGl8X8
pGmR3h++m8c6IQrC0/9Umpv7kIJE0FkozpWfMTSlky3CXU/t9qXrxlOiO+TOOBYT
eFODwUdBQ+HzLxO7WYnUk2NiaRR9xyXbRQWOnGxI3s3ofPY961newRWNBhrLUY1W
aotAqgYQqOVt0CsiYrA/Jf2ISn33l8eothipT6HW/yrodecS9/3UOCfF7DpZ+AS/
Ij54lzjpFzuW0chyPm3mRapFa/Gx2qB1UTeuC4eB8hwUizD1qAEyDm2Fts5Rs9T/
TJCXAK4GGArTVd2RafB1Q1sJBMnI99lUlJB8eBLQfP1GFqAoqQezsmtWgfVqK60r
CKjFayvZfrNdjcXhOqJIvbDx+ZI4YBU4lmVhakchr3HUec4GootuFgCuPNua92d/
45gQJEa+KUryRo5tN+K2RlwCk9d0uUUbIlkCfb61NvvPp+gpEojgUuFGJnDlDnh9
bRzPS3soDSI6w5FECZnpA2pRgh5yDAJ1bNmNb5VXx5VoTlZ+LAOxYvr7V5la/OTL
YTkaYVHv51VyKfQRnJvkQFGh+MRnjaqpU4Xs1czHnS1A3jkoyuVgsgmnhZRFKngY
/PdGp1P3eArCcwxHEYFp1fzulUxftQyJUK/NOFBn2C/x7DBeUu6UxFDUvQVz5fVe
vi9kwpSTl8GDC4Ep+SubDWqDdml7r+ZJYa4BhDRnDAIoTZKcI5yfZZR6wCQK22Dx
AUX2TtaSZNDJ15tdLgK8cEYFmiM3mrPd0H0yeUkzrNraFd9/ppMFNDQHXbRym9Qm
8xV2Bx6/RPdJJkQuklkhye86aWWS6WTOkPLWdAUzBDHhbu/qF0JLAczGZB0i2B6n
C2cxoIg+zDhXWZvkvqFfMlgO8Fk8u9tIn2+e+KoBVT0cz3mxFuYbe1QPiweRQmj0
qLMBJwxhojs0GeWMTIVHsSezv9dLFgepAfR4wSwZIkTN5zkcvqd1lscBVowA9QBz
hKCoXkZVWSSuV2RxBqgZfBmMPS0WtFvAr8LAlSuT263xpCz1+ei9i2uKLp09t1as
vgxuJ2iLxnhQnX9xbp12Gt0fG3aUEKlcvLBVM3/xRyiqBWEkIV8u8Ln1aul0kexm
WPTjiY+HGMVmsPr6MHqcQvVka1fWimieTaJ55TGbEnoVSUuQv6nuPDO/8LgH1+gw
n86qAilNYFJKciZXFoi09LzsJlAmcZTgHWnwZpSXOpPUM5khcUpBGxuceW7wJ2S+
qbjwUYBzrswxpqOBvRN/VVfCB6qawuV8SijeNPpXQVFnEYS4atDwaNqQe8f00vG5
t1cILwipKSItiasf33zcVxa6sqBfL6gMiztmA8HGfBlBHKDxRBTCb3x3gzuYNW7J
38f/whR2/mojcsvmWrFeTxpfCgbo8I2cYwOVB2cnCunz6NNa9ggcGwBMloq4PPh8
SB+przwRkf9pNCA1yg8=
-----END ENCRYPTED PRIVATE KEY-----`

// decodeBlock is a small test helper — decodes a PEM string into a *pem.Block.
func decodeBlock(t *testing.T, pemStr string) *pem.Block {
	t.Helper()
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		t.Fatal("failed to decode PEM constant")
	}
	return block
}

func blockWithPBKDF2Params(t *testing.T, pemStr string, update func(*pbkdf2Params)) *pem.Block {
	t.Helper()
	block := decodeBlock(t, pemStr)

	var encInfo encryptedPrivateKeyInfo
	if _, err := asn1.Unmarshal(block.Bytes, &encInfo); err != nil {
		t.Fatalf("failed to parse EncryptedPrivateKeyInfo: %v", err)
	}

	var pbes2 pbes2Params
	if _, err := asn1.Unmarshal(encInfo.Algorithm.Params.FullBytes, &pbes2); err != nil {
		t.Fatalf("failed to parse PBES2 params: %v", err)
	}

	var kdf pbkdf2Params
	if _, err := asn1.Unmarshal(pbes2.KDF.Params.FullBytes, &kdf); err != nil {
		t.Fatalf("failed to parse PBKDF2 params: %v", err)
	}

	update(&kdf)

	kdfBytes, err := asn1.Marshal(kdf)
	if err != nil {
		t.Fatalf("failed to marshal PBKDF2 params: %v", err)
	}
	pbes2.KDF.Params = asn1.RawValue{FullBytes: kdfBytes}

	pbes2Bytes, err := asn1.Marshal(pbes2)
	if err != nil {
		t.Fatalf("failed to marshal PBES2 params: %v", err)
	}
	encInfo.Algorithm.Params = asn1.RawValue{FullBytes: pbes2Bytes}

	encoded, err := asn1.Marshal(encInfo)
	if err != nil {
		t.Fatalf("failed to marshal EncryptedPrivateKeyInfo: %v", err)
	}

	return &pem.Block{Type: block.Type, Headers: block.Headers, Bytes: encoded}
}

// ---------------------------------------------------------------------------
// parsePKCS8EncryptedPrivateKey — end-to-end tests using OpenSSL-generated PEMs
// ---------------------------------------------------------------------------

func TestParsePKCS8EncryptedPrivateKey_HappyPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		pemStr string
	}{
		{"AES-256-CBC", pemAES256},
		{"AES-192-CBC", pemAES192},
		{"AES-128-CBC", pemAES128},
		{"3DES-CBC", pemDES3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			block := decodeBlock(t, tc.pemStr)
			key, err := parsePKCS8EncryptedPrivateKey(block, []byte(testKey))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if _, ok := key.(*rsa.PrivateKey); !ok {
				t.Fatalf("expected *rsa.PrivateKey, got %T", key)
			}
		})
	}
}

func TestParsePKCS8EncryptedPrivateKey_NilBlock(t *testing.T) {
	t.Parallel()
	_, err := parsePKCS8EncryptedPrivateKey(nil, []byte(testKey))
	if err == nil {
		t.Fatal("expected error for nil block, got nil")
	}
}

func TestParsePKCS8EncryptedPrivateKey_WrongBlockType(t *testing.T) {
	t.Parallel()
	block := &pem.Block{Type: "CERTIFICATE", Bytes: []byte{0x30, 0x00}}
	_, err := parsePKCS8EncryptedPrivateKey(block, []byte(testKey))
	if err == nil {
		t.Fatal("expected error for wrong block type, got nil")
	}
}

func TestParsePKCS8EncryptedPrivateKey_WrongKey(t *testing.T) {
	t.Parallel()
	// A wrong key should be caught early by padding validation,
	// producing a clear padding validation error rather than a generic ASN.1 error.
	block := decodeBlock(t, pemAES256)
	_, err := parsePKCS8EncryptedPrivateKey(block, []byte("wrongkey"))
	if err == nil {
		t.Fatal("expected error for wrong key, got nil")
	}
}

func TestParsePKCS8EncryptedPrivateKey_CorruptedBytes(t *testing.T) {
	t.Parallel()
	block := decodeBlock(t, pemAES256)
	// Flip some bytes in the middle of the DER payload to simulate corruption.
	for i := 10; i < 20; i++ {
		block.Bytes[i] ^= 0xFF
	}
	_, err := parsePKCS8EncryptedPrivateKey(block, []byte(testKey))
	if err == nil {
		t.Fatal("expected error for corrupted block bytes, got nil")
	}
}

func TestParsePKCS8EncryptedPrivateKey_UnsupportedEncryptionAlgorithmOID(t *testing.T) {
	t.Parallel()
	// pemPBES1 uses a PBES1 outer scheme (pbeWithSHA1And3-KeyTripleDES-CBC),
	// so its top-level OID is not oidPBES2 and the parser must reject it
	// at the outer algorithm OID check.
	block := decodeBlock(t, pemPBES1)
	_, err := parsePKCS8EncryptedPrivateKey(block, []byte(testKey))
	if err == nil {
		t.Fatal("expected error for unsupported outer encryption algorithm OID, got nil")
	}
}

func TestParsePKCS8EncryptedPrivateKey_RejectsUnsafePBKDF2Params(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		edit func(*pbkdf2Params)
		want string
	}{
		{
			name: "iteration count too high",
			edit: func(kdf *pbkdf2Params) {
				kdf.Iterations = maxPBKDF2Iterations + 1
			},
			want: "invalid PBKDF2 iteration count",
		},
		{
			name: "salt too large",
			edit: func(kdf *pbkdf2Params) {
				kdf.Salt = make([]byte, maxPBKDF2SaltLength+1)
			},
			want: "PBKDF2 salt length",
		},
		{
			name: "explicit key length does not match cipher",
			edit: func(kdf *pbkdf2Params) {
				kdf.KeyLength = 64
			},
			want: "does not match cipher key length",
		},
		{
			name: "explicit key length negative",
			edit: func(kdf *pbkdf2Params) {
				kdf.KeyLength = -1
			},
			want: "invalid PBKDF2 key length",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			block := blockWithPBKDF2Params(t, pemAES256, tc.edit)
			_, err := parsePKCS8EncryptedPrivateKey(block, []byte(testKey))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestParsePKCS8EncryptedPrivateKey_AcceptsMatchingExplicitKeyLength(t *testing.T) {
	t.Parallel()
	block := blockWithPBKDF2Params(t, pemAES256, func(kdf *pbkdf2Params) {
		kdf.KeyLength = 32
	})

	key, err := parsePKCS8EncryptedPrivateKey(block, []byte(testKey))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := key.(*rsa.PrivateKey); !ok {
		t.Fatalf("expected *rsa.PrivateKey, got %T", key)
	}
}

// ---------------------------------------------------------------------------
// stripPKCS7Padding
// ---------------------------------------------------------------------------

func TestStripPKCS7Padding_Valid(t *testing.T) {
	t.Parallel()
	// Build a block where the last 4 bytes are the pad value 0x04.
	data := append([]byte("HELLO!!!"), 0x04, 0x04, 0x04, 0x04) // 12 bytes, blockSize=4
	got, err := stripPKCS7Padding(data, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "HELLO!!!" {
		t.Fatalf("unexpected result: %q", got)
	}
}

func TestStripPKCS7Padding_PadByteZero(t *testing.T) {
	t.Parallel()
	// Pad byte value of 0 is explicitly invalid per PKCS#7.
	data := []byte{0x01, 0x02, 0x03, 0x00}
	_, err := stripPKCS7Padding(data, 4)
	if err == nil {
		t.Fatal("expected error for pad byte 0, got nil")
	}
}

func TestStripPKCS7Padding_PadByteExceedsBlockSize(t *testing.T) {
	t.Parallel()
	// Last byte claims padding of 5 but block size is 4 — invalid.
	data := []byte{0x01, 0x02, 0x03, 0x05}
	_, err := stripPKCS7Padding(data, 4)
	if err == nil {
		t.Fatal("expected error when pad byte exceeds block size, got nil")
	}
}

func TestStripPKCS7Padding_InconsistentPadBytes(t *testing.T) {
	t.Parallel()
	// Last byte says pad length 3, but the preceding bytes don't match.
	data := []byte{0x01, 0x02, 0x01, 0x03}
	_, err := stripPKCS7Padding(data, 4)
	if err == nil {
		t.Fatal("expected error for inconsistent pad bytes, got nil")
	}
}

func TestStripPKCS7Padding_EmptyData(t *testing.T) {
	t.Parallel()
	_, err := stripPKCS7Padding([]byte{}, 16)
	if err == nil {
		t.Fatal("expected error for empty data, got nil")
	}
}

// ---------------------------------------------------------------------------
// hashForPBKDF2PRF
// ---------------------------------------------------------------------------

func TestHashForPBKDF2PRF(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		oid         asn1.ObjectIdentifier
		expectError bool
	}{
		// Empty OID must default to SHA-1 per PKCS#5.
		{"default (empty OID)", asn1.ObjectIdentifier{}, false},
		{"HMAC-SHA1", oidHMACWithSHA1, false},
		{"HMAC-SHA256", oidHMACWithSHA256, false},
		{"HMAC-SHA384", oidHMACWithSHA384, false},
		{"HMAC-SHA512", oidHMACWithSHA512, false},
		{"unsupported OID", asn1.ObjectIdentifier{9, 9, 9, 9}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prf := algorithmIdentifier{OID: tc.oid}
			fn, err := hashForPBKDF2PRF(prf)
			if tc.expectError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if fn == nil {
				t.Fatal("expected non-nil hash constructor")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// keyLengthForOID
// ---------------------------------------------------------------------------

func TestKeyLengthForOID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		oid         asn1.ObjectIdentifier
		expected    int
		expectError bool
	}{
		{"AES-128", oidAES128CBC, 16, false},
		{"AES-192", oidAES192CBC, 24, false},
		{"AES-256", oidAES256CBC, 32, false},
		{"3DES", oidDESEDE3, 24, false},
		{"unsupported", asn1.ObjectIdentifier{9, 9, 9, 9}, 0, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := keyLengthForOID(tc.oid)
			if tc.expectError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.expected {
				t.Fatalf("expected %d, got %d", tc.expected, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// decryptCBC
// ---------------------------------------------------------------------------

func TestDecryptCBC_AES_RoundTrip(t *testing.T) {
	t.Parallel()
	// Encrypt a known plaintext with AES-128-CBC manually, then decrypt it.
	key := make([]byte, 16) // 16-byte AES-128 key (all zeros for test)
	iv := make([]byte, 16)  // 16-byte IV (all zeros for test)

	// Plaintext: "HELLO" + PKCS#7 padding to 16 bytes (pad value = 11).
	plaintext := []byte("HELLO\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b\x0b")

	// Encrypt using standard library directly.
	block, _ := aes.NewCipher(key)
	ciphertext := make([]byte, len(plaintext))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, plaintext)

	got, err := decryptCBC(key, iv, ciphertext, aes.NewCipher, aes.BlockSize)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "HELLO" {
		t.Fatalf("expected %q, got %q", "HELLO", got)
	}
}

func TestDecrypt_UnsupportedOID(t *testing.T) {
	t.Parallel()
	// Use a valid ASN.1 object identifier that is not one of the supported
	// AES-CBC or 3DES-CBC encryption scheme OIDs.
	oid := asn1.ObjectIdentifier{1, 2, 3, 4}

	// The decrypt function checks the OID before attempting any cipher work,
	// so the actual key/IV/ciphertext contents are irrelevant for this branch.
	_, err := decrypt(oid, []byte{0x00}, []byte{0x00}, []byte{0x00})
	if err == nil {
		t.Fatal("expected error for unsupported encryption OID, got nil")
	}
}

func TestDecryptCBC_IVLengthMismatch(t *testing.T) {
	t.Parallel()
	key := make([]byte, 16)
	shortIV := make([]byte, 8) // wrong: AES needs 16
	ciphertext := make([]byte, 16)
	_, err := decryptCBC(key, shortIV, ciphertext, aes.NewCipher, aes.BlockSize)
	if err == nil {
		t.Fatal("expected error for IV length mismatch, got nil")
	}
}

func TestDecryptCBC_EmptyCiphertext(t *testing.T) {
	t.Parallel()
	key := make([]byte, 16)
	iv := make([]byte, 16)
	_, err := decryptCBC(key, iv, []byte{}, aes.NewCipher, aes.BlockSize)
	if err == nil {
		t.Fatal("expected error for empty ciphertext, got nil")
	}
}

func TestDecryptCBC_CiphertextNotMultipleOfBlockSize(t *testing.T) {
	t.Parallel()
	key := make([]byte, 16)
	iv := make([]byte, 16)
	ciphertext := make([]byte, 17) // 17 is not a multiple of 16
	_, err := decryptCBC(key, iv, ciphertext, aes.NewCipher, aes.BlockSize)
	if err == nil {
		t.Fatal("expected error for ciphertext not multiple of block size, got nil")
	}
}

func TestDecryptCBC_InvalidKey(t *testing.T) {
	t.Parallel()
	// aes.NewCipher rejects keys that aren't 16, 24, or 32 bytes.
	badKey := make([]byte, 7)
	iv := make([]byte, aes.BlockSize)
	ciphertext := make([]byte, aes.BlockSize)
	_, err := decryptCBC(badKey, iv, ciphertext, aes.NewCipher, aes.BlockSize)
	if err == nil {
		t.Fatal("expected error for invalid key length, got nil")
	}
}

func TestDecryptCBC_3DES_RoundTrip(t *testing.T) {
	t.Parallel()
	// Same round-trip test as AES but using 3DES to cover the des path.
	key := make([]byte, 24)           // 24-byte 3DES key
	iv := make([]byte, des.BlockSize) // 8-byte IV

	// Plaintext: "HI" + PKCS#7 padding to 8 bytes (pad value = 6).
	plaintext := []byte("HI\x06\x06\x06\x06\x06\x06")

	block, _ := des.NewTripleDESCipher(key)
	ciphertext := make([]byte, len(plaintext))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, plaintext)

	got, err := decryptCBC(key, iv, ciphertext, des.NewTripleDESCipher, des.BlockSize)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "HI" {
		t.Fatalf("expected %q, got %q", "HI", got)
	}
}

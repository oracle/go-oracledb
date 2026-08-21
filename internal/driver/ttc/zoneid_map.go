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

package ttc

import "sync"

// zoneIDFromName holds a (partial) mapping of IANA time zone names to Oracle region IDs.
// This table is seeded with a subset of frequently used zones and those provided in the
// spec. It can be extended over time. The reverse map is cached lazily.
var zoneIDFromName = map[string]int{
	// Africa
	"Africa/Abidjan":       42,
	"Africa/Accra":         50,
	"Africa/Addis_Ababa":   47,
	"Africa/Algiers":       30,
	"Africa/Asmara":        46,
	"Africa/Asmera":        558,
	"Africa/Bamako":        58,
	"Africa/Bangui":        37,
	"Africa/Banjul":        49,
	"Africa/Bissau":        52,
	"Africa/Blantyre":      57,
	"Africa/Brazzaville":   41,
	"Africa/Bujumbura":     35,
	"Africa/Cairo":         44,
	"Africa/Casablanca":    61,
	"Africa/Conakry":       51,
	"Africa/Dakar":         69,
	"Africa/Dar_es_Salaam": 75,
	"Africa/Djibouti":      43,
	"Africa/Douala":        36,
	"Africa/El_Aaiun":      62,
	"Africa/Freetown":      70,
	"Africa/Gaborone":      33,
	"Africa/Harare":        80,
	"Africa/Johannesburg":  72,
	"Africa/Juba":          321,
	"Africa/Kampala":       78,
	"Africa/Khartoum":      73,
	"Africa/Kigali":        67,
	"Africa/Kinshasa":      39,
	"Africa/Lagos":         66,
	"Africa/Libreville":    48,
	"Africa/Lome":          76,
	"Africa/Luanda":        31,
	"Africa/Lubumbashi":    40,
	"Africa/Lusaka":        79,
	"Africa/Malabo":        45,
	"Africa/Maputo":        63,
	"Africa/Maseru":        54,
	"Africa/Mbabane":       74,
	"Africa/Mogadishu":     71,
	"Africa/Monrovia":      55,
	"Africa/Nairobi":       53,
	"Africa/Ndjamena":      38,
	"Africa/Niamey":        65,
	"Africa/Nouakchott":    60,
	"Africa/Ouagadougou":   34,
	"Africa/Porto-Novo":    32,
	"Africa/Sao_Tome":      68,
	"Africa/Timbuktu":      570,
	"Africa/Tripoli":       56,
	"Africa/Tunis":         77,
	"Africa/Windhoek":      64,

	// America (subset)
	"America/Adak":          108,
	"America/Anchorage":     106,
	"America/Antigua":       147,
	"America/Aruba":         181,
	"America/Asuncion":      200,
	"America/Bahia":         90,
	"America/Barbados":      149,
	"America/Belize":        150,
	"America/Bogota":        195,
	"America/Buenos_Aires":  175,
	"America/Chicago":       101,
	"America/Denver":        102,
	"America/Detroit":       116,
	"America/Edmonton":      129,
	"America/Grand_Turk":    172,
	"America/Guatemala":     159,
	"America/Guyana":        199,
	"America/Halifax":       120,
	"America/Havana":        153,
	"America/Indianapolis":  111,
	"America/Jamaica":       162,
	"America/Los_Angeles":   103,
	"America/Manaus":        192,
	"America/Mazatlan":      144,
	"America/Mexico_City":   141,
	"America/Moncton":       92,
	"America/Monterrey":     227,
	"America/Montevideo":    204,
	"America/Montreal":      122,
	"America/New_York":      100,
	"America/Panama":        166,
	"America/Paramaribo":    202,
	"America/Phoenix":       109,
	"America/Port_of_Spain": 203,
	"America/Puerto_Rico":   167,
	"America/Santiago":      194,
	"America/Santo_Domingo": 155,
	"America/Sao_Paulo":     188,
	"America/St_Johns":      118,
	"America/Tijuana":       145,
	"America/Toronto":       220,
	"America/Vancouver":     130,
	"America/Winnipeg":      126,
	"America/Yakutat":       105,

	// Arctic / Antarctica (subset)
	"Arctic/Longyearbyen":       909,
	"Antarctica/Casey":          230,
	"Antarctica/Davis":          231,
	"Antarctica/DumontDUrville": 233,
	"Antarctica/Mawson":         232,
	"Antarctica/McMurdo":        236,
	"Antarctica/Macquarie":      85,
	"Antarctica/Palmer":         235,
	"Antarctica/Rothera":        82,
	"Antarctica/Syowa":          234,
	"Antarctica/Troll":          329,
	"Antarctica/Vostok":         59,

	// Asia (subset)
	"Asia/Aden":          302,
	"Asia/Amman":         268,
	"Asia/Anadyr":        312,
	"Asia/Aqtau":         271,
	"Asia/Aqtobe":        270,
	"Asia/Ashgabat":      297,
	"Asia/Baghdad":       265,
	"Asia/Bahrain":       243,
	"Asia/Baku":          242,
	"Asia/Bangkok":       296,
	"Asia/Beirut":        277,
	"Asia/Brunei":        246,
	"Asia/Calcutta":      260, // legacy alias
	"Asia/Colombo":       293,
	"Asia/Damascus":      294,
	"Asia/Dhaka":         756,
	"Asia/Dili":          259,
	"Asia/Dubai":         298,
	"Asia/Hebron":        324,
	"Asia/Hong_Kong":     254,
	"Asia/Jakarta":       261,
	"Asia/Jerusalem":     266,
	"Asia/Kabul":         240,
	"Asia/Kamchatka":     311,
	"Asia/Karachi":       284,
	"Asia/Kathmandu":     797,
	"Asia/Kolkata":       772,
	"Asia/Krasnoyarsk":   306,
	"Asia/Kuala_Lumpur":  278,
	"Asia/Kuwait":        275,
	"Asia/Macau":         768,
	"Asia/Magadan":       310,
	"Asia/Manila":        286,
	"Asia/Muscat":        283,
	"Asia/Nicosia":       257,
	"Asia/Novosibirsk":   305,
	"Asia/Qatar":         287,
	"Asia/Riyadh":        288,
	"Asia/Seoul":         273,
	"Asia/Shanghai":      250,
	"Asia/Singapore":     292,
	"Asia/Taipei":        255,
	"Asia/Tashkent":      300,
	"Asia/Tehran":        264,
	"Asia/Tokyo":         267,
	"Asia/Vladivostok":   309,
	"Asia/Yekaterinburg": 303,
	"Asia/Yerevan":       241,

	// Atlantic (subset)
	"Atlantic/Azores":        336,
	"Atlantic/Bermuda":       330,
	"Atlantic/Canary":        338,
	"Atlantic/Cape_Verde":    339,
	"Atlantic/Reykjavik":     334,
	"Atlantic/South_Georgia": 332,

	// Australia (subset)
	"Australia/Adelaide":  349,
	"Australia/Brisbane":  347,
	"Australia/Darwin":    345,
	"Australia/Eucla":     356,
	"Australia/Hobart":    350,
	"Australia/Melbourne": 351,
	"Australia/Perth":     346,
	"Australia/Sydney":    352,

	// Europe (subset)
	"Europe/Amsterdam":   396,
	"Europe/Andorra":     373,
	"Europe/Athens":      385,
	"Europe/Belgrade":    412,
	"Europe/Berlin":      383,
	"Europe/Brussels":    376,
	"Europe/Bucharest":   400,
	"Europe/Budapest":    386,
	"Europe/Copenhagen":  379,
	"Europe/Dublin":      371,
	"Europe/Gibraltar":   384,
	"Europe/Helsinki":    381,
	"Europe/Istanbul":    407,
	"Europe/Kaliningrad": 401,
	"Europe/Kiev":        408,
	"Europe/Kyiv":        417,
	"Europe/Lisbon":      399,
	"Europe/London":      369,
	"Europe/Luxembourg":  391,
	"Europe/Madrid":      404,
	"Europe/Malta":       392,
	"Europe/Minsk":       375,
	"Europe/Monaco":      395,
	"Europe/Moscow":      402,
	"Europe/Oslo":        397,
	"Europe/Paris":       382,
	"Europe/Prague":      378,
	"Europe/Riga":        388,
	"Europe/Rome":        387,
	"Europe/Sofia":       377,
	"Europe/Stockholm":   405,
	"Europe/Tallinn":     380,
	"Europe/Vienna":      374,
	"Europe/Vilnius":     390,
	"Europe/Warsaw":      398,
	"Europe/Zurich":      406,

	// Indian (subset)
	"Indian/Chagos":    436,
	"Indian/Christmas": 439,
	"Indian/Cocos":     440,
	"Indian/Mahe":      442,
	"Indian/Maldives":  437,
	"Indian/Mauritius": 443,
	"Indian/Reunion":   445,

	// Pacific (subset)
	"Pacific/Apia":         479,
	"Pacific/Auckland":     471,
	"Pacific/Chatham":      472,
	"Pacific/Easter":       451,
	"Pacific/Fiji":         454,
	"Pacific/Guam":         458,
	"Pacific/Honolulu":     450,
	"Pacific/Kiritimati":   461,
	"Pacific/Majuro":       463,
	"Pacific/Marquesas":    456,
	"Pacific/Noumea":       470,
	"Pacific/Pago_Pago":    478,
	"Pacific/Port_Moresby": 476,
	"Pacific/Tahiti":       457,
	"Pacific/Tongatapu":    483,

	// Standard / abbreviations (subset)
	"EET": 368,
	"GMT": 513,
	"UTC": 5121,
	"MET": 367,
	"MST": 212,
	"HST": 213,
	"PST": 2151,
	"EST": 211,
	"CST": 1637,
}

var (
	zoneNameFromIDCacheMu sync.RWMutex
	zoneNameFromIDCache   = make(map[int]string)
)

// getZoneFromID returns the IANA time zone name for a given Oracle region id.
// The boolean indicates whether a mapping was found.
func getZoneFromID(id int) (string, bool) {
	zoneNameFromIDCacheMu.RLock()
	if n, exists := zoneNameFromIDCache[id]; exists {
		zoneNameFromIDCacheMu.RUnlock()
		return n, true
	}
	zoneNameFromIDCacheMu.RUnlock()

	zoneNameFromIDCacheMu.Lock()
	defer zoneNameFromIDCacheMu.Unlock()

	if n, exists := zoneNameFromIDCache[id]; exists {
		return n, true
	}

	for k, v := range zoneIDFromName {
		if v == id {
			zoneNameFromIDCache[v] = k
			return k, true
		}
	}
	return "", false
}

// getIDFromZone returns the Oracle region id for a given IANA time zone name.
// The boolean indicates whether a mapping was found. Matching is case sensitive to
// honor canonical names; pass the exact zone string (e.g., "America/Los_Angeles").
func getIDFromZone(zone string) (int, bool) {
	if id, exists := zoneIDFromName[zone]; exists {
		return id, exists
	}
	return -1, false
}

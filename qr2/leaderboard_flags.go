package qr2

// Wii country codes for the countries Mario Kart Wii has a flag for (its national_flag
// layout names each flag pane by this code), by ISO 3166 code as MKW Lounge gives them.
// Countries without a game flag aren't listed: the leaderboard shows no flag for them.
var wiiFlagCountries = map[string]uint16{
	"JP": 1, "KR": 136, "US": 49, "CA": 18, "MX": 36,
	"AI": 8, "AG": 9, "AR": 10, "AW": 11, "BS": 12, "BB": 13, "BZ": 14, "BO": 15, "BR": 16,
	"VG": 17, "KY": 19, "CL": 20, "CO": 21, "CR": 22, "DM": 23, "DO": 24, "EC": 25, "SV": 26,
	"GD": 28, "GT": 30, "GY": 31, "HT": 32, "HN": 33, "JM": 34, "MS": 37, "AN": 38, "NI": 39,
	"PA": 40, "PY": 41, "PE": 42, "KN": 43, "LC": 44, "VC": 45, "SR": 46, "TT": 47, "TC": 48,
	"UY": 50, "VI": 51, "VE": 52,
	"AU": 65, "IE": 82, "NZ": 95, "GB": 110, "UK": 110,
	"AT": 66, "DE": 78, "CH": 108, "BE": 67, "FR": 77, "LU": 88, "IT": 83, "PT": 98, "ES": 105,
	"NL": 94, "DK": 74, "FI": 76, "GR": 79, "NO": 96, "SE": 107,
}

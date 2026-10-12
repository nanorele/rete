package faker

import (
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"
)

func IsDynamic(name string) bool {
	_, ok := generators[name]
	return ok
}

func Lookup(name string) (string, bool) {
	g, ok := generators[name]
	if !ok {
		return "", false
	}
	return g(), true
}

func Names() []string {
	out := make([]string, 0, len(names))
	out = append(out, names...)
	return out
}

func Describe(name string) string {
	return descriptions[name]
}

var generators map[string]func() string

var names []string

var descriptions = map[string]string{}

func reg(name, desc string, g func() string) {
	generators[name] = g
	descriptions[name] = desc
	names = append(names, name)
}

func pick(list []string) string {
	return list[rand.IntN(len(list))]
}

func pickF(list []string) func() string {
	return func() string { return pick(list) }
}

func digits(n int) string {
	var b strings.Builder
	b.Grow(n)
	for i := 0; i < n; i++ {
		b.WriteByte(byte('0' + rand.IntN(10)))
	}
	return b.String()
}

func alnum(n int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	var b strings.Builder
	b.Grow(n)
	for i := 0; i < n; i++ {
		b.WriteByte(chars[rand.IntN(len(chars))])
	}
	return b.String()
}

func upper(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteByte(byte('A' + rand.IntN(26)))
	}
	return b.String()
}

func uuid4() string {
	var b [16]byte
	if _, err := crand.Read(b[:]); err != nil {
		for i := range b {
			b[i] = byte(randInt(256))
		}
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

func randInt(n int) int {
	return rand.IntN(n)
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func loremWords(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = pick(lorem)
	}
	return strings.Join(parts, " ")
}

func loremSentence() string {
	return capitalize(loremWords(4+randInt(9))) + "."
}

func loremSentences(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = loremSentence()
	}
	return strings.Join(parts, " ")
}

func loremParagraph() string {
	return loremSentences(3 + randInt(4))
}

func jsDate(t time.Time) string {
	name, off := t.Zone()
	sign := "+"
	if off < 0 {
		sign = "-"
		off = -off
	}
	return fmt.Sprintf("%s GMT%s%02d%02d (%s)", t.Format("Mon Jan 02 2006 15:04:05"), sign, off/3600, (off%3600)/60, name)
}

func fullName() string {
	return pick(firstNames) + " " + pick(lastNames)
}

func domainWord() string {
	return strings.ToLower(pick(firstNames))
}

func domainName() string {
	return domainWord() + "." + pick(domainSuffixes)
}

func userName() string {
	switch randInt(3) {
	case 0:
		return pick(firstNames) + "." + pick(lastNames)
	case 1:
		return pick(firstNames) + "_" + pick(lastNames) + digits(2)
	default:
		return pick(firstNames) + digits(2)
	}
}

func fileExtPick(exts []string) string {
	return pick(exts)
}

func init() {
	generators = map[string]func() string{}

	reg("$guid", "A uuid-v4 style guid", uuid4)
	reg("$timestamp", "The current UNIX timestamp in seconds", func() string { return strconv.FormatInt(time.Now().Unix(), 10) })
	reg("$isoTimestamp", "The current ISO timestamp at zero UTC", func() string { return time.Now().UTC().Format("2006-01-02T15:04:05.000Z") })
	reg("$randomUUID", "A random 36-character UUID", uuid4)

	reg("$randomAlphaNumeric", "A random alpha-numeric character", func() string { return alnum(1) })
	reg("$randomBoolean", "A random boolean value", func() string { return pick([]string{"true", "false"}) })
	reg("$randomInt", "A random integer between 0 and 1000", func() string { return strconv.Itoa(randInt(1001)) })
	reg("$randomColor", "A random color", pickF(colors))
	reg("$randomHexColor", "A random hex value", func() string {
		return "#" + hex.EncodeToString([]byte{byte(randInt(256)), byte(randInt(256)), byte(randInt(256))})
	})
	reg("$randomAbbreviation", "A random abbreviation", pickF(abbreviations))

	reg("$randomIP", "A random IPv4 address", func() string {
		return fmt.Sprintf("%d.%d.%d.%d", 1+randInt(254), randInt(256), randInt(256), 1+randInt(254))
	})
	reg("$randomIPV6", "A random IPv6 address", func() string {
		parts := make([]string, 8)
		for i := range parts {
			parts[i] = fmt.Sprintf("%04x", randInt(65536))
		}
		return strings.Join(parts, ":")
	})
	reg("$randomMACAddress", "A random MAC address", func() string {
		parts := make([]string, 6)
		for i := range parts {
			parts[i] = fmt.Sprintf("%02x", randInt(256))
		}
		return strings.Join(parts, ":")
	})
	reg("$randomPassword", "A random 15-character alpha-numeric password", func() string { return alnum(15) })
	reg("$randomLocale", "A random two-letter language code (ISO 639-1)", pickF(locales))
	reg("$randomUserAgent", "A random user agent", pickF(userAgents))
	reg("$randomProtocol", "A random internet protocol", func() string { return pick([]string{"http", "https"}) })
	reg("$randomSemver", "A random semantic version number", func() string {
		return fmt.Sprintf("%d.%d.%d", randInt(10), randInt(10), randInt(10))
	})

	reg("$randomFirstName", "A random first name", pickF(firstNames))
	reg("$randomLastName", "A random last name", pickF(lastNames))
	reg("$randomFullName", "A random first and last name", fullName)
	reg("$randomNamePrefix", "A random name prefix", pickF([]string{"Mr.", "Mrs.", "Ms.", "Miss", "Dr."}))
	reg("$randomNameSuffix", "A random name suffix", pickF([]string{"Jr.", "Sr.", "I", "II", "III", "IV", "V", "MD", "DDS", "PhD", "DVM"}))

	reg("$randomJobArea", "A random job area", pickF(jobAreas))
	reg("$randomJobDescriptor", "A random job descriptor", pickF(jobDescriptors))
	reg("$randomJobTitle", "A random job title", func() string {
		return pick(jobDescriptors) + " " + pick(jobAreas) + " " + pick(jobTypes)
	})
	reg("$randomJobType", "A random job type", pickF(jobTypes))

	reg("$randomPhoneNumber", "A random ten-digit phone number", func() string {
		return digits(3) + "-" + digits(3) + "-" + digits(4)
	})
	reg("$randomPhoneNumberExt", "A random phone number with extension (12 digits)", func() string {
		return digits(2) + "-" + digits(3) + "-" + digits(3) + "-" + digits(4)
	})
	reg("$randomCity", "A random city name", func() string {
		switch randInt(3) {
		case 0:
			return pick(cityPrefixes) + " " + pick(lastNames) + pick(citySuffixes)
		case 1:
			return pick(firstNames) + pick(citySuffixes)
		default:
			return pick(lastNames) + pick(citySuffixes)
		}
	})
	reg("$randomStreetName", "A random street name", func() string {
		if randInt(2) == 0 {
			return pick(firstNames) + " " + pick(streetSuffixes)
		}
		return pick(lastNames) + " " + pick(streetSuffixes)
	})
	reg("$randomStreetAddress", "A random street address", func() string {
		return digits(3+randInt(3)) + " " + pick(lastNames) + " " + pick(streetSuffixes)
	})
	reg("$randomCountry", "A random country", pickF(countries))
	reg("$randomCountryCode", "A random two-letter country code (ISO 3166-1 alpha-2)", pickF(countryCodes))
	reg("$randomLatitude", "A random latitude coordinate", func() string {
		return strconv.FormatFloat(rand.Float64()*180-90, 'f', 4, 64)
	})
	reg("$randomLongitude", "A random longitude coordinate", func() string {
		return strconv.FormatFloat(rand.Float64()*360-180, 'f', 4, 64)
	})

	reg("$randomAvatarImage", "A random avatar image", func() string {
		return "https://cloudflare-ipfs.com/ipfs/Qmd3W5DuhgHirLHGVixi6V76LhCkZUz6pnFt5AJBiyvHye/avatar/" + strconv.Itoa(randInt(1250)) + ".jpg"
	})
	imageURL := func(category string) func() string {
		return func() string {
			if category == "" {
				return "https://loremflickr.com/640/480"
			}
			return "https://loremflickr.com/640/480/" + category
		}
	}
	reg("$randomImageUrl", "A URL of a random image", imageURL(""))
	reg("$randomAbstractImage", "A URL of a random abstract image", imageURL("abstract"))
	reg("$randomAnimalsImage", "A URL of a random animal image", imageURL("animals"))
	reg("$randomBusinessImage", "A URL of a random stock business image", imageURL("business"))
	reg("$randomCatsImage", "A URL of a random cat image", imageURL("cats"))
	reg("$randomCityImage", "A URL of a random city image", imageURL("city"))
	reg("$randomFoodImage", "A URL of a random food image", imageURL("food"))
	reg("$randomNightlifeImage", "A URL of a random nightlife image", imageURL("nightlife"))
	reg("$randomFashionImage", "A URL of a random fashion image", imageURL("fashion"))
	reg("$randomPeopleImage", "A URL of a random image of a person", imageURL("people"))
	reg("$randomNatureImage", "A URL of a random nature image", imageURL("nature"))
	reg("$randomSportsImage", "A URL of a random sports image", imageURL("sports"))
	reg("$randomTransportImage", "A URL of a random transportation image", imageURL("transport"))
	reg("$randomImageDataUri", "A random image data URI", func() string {
		c := fmt.Sprintf("%%23%02x%02x%02x", randInt(256), randInt(256), randInt(256))
		return "data:image/svg+xml;charset=UTF-8,%3Csvg%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20version%3D%221.1%22%20baseProfile%3D%22full%22%20width%3D%22640%22%20height%3D%22480%22%3E%3Crect%20width%3D%22100%25%22%20height%3D%22100%25%22%20fill%3D%22" + c + "%22%2F%3E%3C%2Fsvg%3E"
	})

	reg("$randomBankAccount", "A random 8-digit bank account number", func() string { return digits(8) })
	reg("$randomBankAccountName", "A random bank account name", pickF([]string{"Checking Account", "Savings Account", "Money Market Account", "Investment Account", "Home Loan Account", "Credit Card Account", "Auto Loan Account", "Personal Loan Account"}))
	reg("$randomCreditCardMask", "A random masked credit card number", func() string { return digits(4) })
	reg("$randomBankAccountBic", "A random BIC (Bank Identifier Code)", func() string { return upper(4) + pick(countryCodes) + upper(1) + "1" })
	reg("$randomBankAccountIban", "A random 15-31 character IBAN (International Bank Account Number)", func() string {
		return pick(countryCodes) + digits(2) + upper(4) + digits(10+randInt(13))
	})
	reg("$randomTransactionType", "A random transaction type", pickF([]string{"deposit", "withdrawal", "payment", "invoice"}))
	reg("$randomCurrencyCode", "A random 3-letter currency code (ISO-4217)", func() string { return currencies[randInt(len(currencies))].code })
	reg("$randomCurrencyName", "A random currency name", func() string { return currencies[randInt(len(currencies))].name })
	reg("$randomCurrencySymbol", "A random currency symbol", func() string { return currencies[randInt(len(currencies))].symbol })
	reg("$randomBitcoin", "A random bitcoin address", func() string {
		const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ123456789"
		n := 26 + randInt(9)
		var b strings.Builder
		b.WriteString(pick([]string{"1", "3"}))
		for i := 0; i < n; i++ {
			b.WriteByte(chars[randInt(len(chars))])
		}
		return b.String()
	})

	reg("$randomCompanyName", "A random company name", func() string {
		switch randInt(3) {
		case 0:
			return pick(lastNames) + " " + pick(companySuffixes)
		case 1:
			return pick(lastNames) + " - " + pick(lastNames)
		default:
			return pick(lastNames) + ", " + pick(lastNames) + " and " + pick(lastNames)
		}
	})
	reg("$randomCompanySuffix", "A random company suffix", pickF(companySuffixes))
	reg("$randomBs", "A random phrase of business-speak", func() string {
		return pick(bsBuzz) + " " + pick(bsAdjectives) + " " + pick(bsNouns)
	})
	reg("$randomBsAdjective", "A random business-speak adjective", pickF(bsAdjectives))
	reg("$randomBsBuzz", "A random business-speak buzzword", pickF(bsBuzz))
	reg("$randomBsNoun", "A random business-speak noun", pickF(bsNouns))
	reg("$randomCatchPhrase", "A random catchphrase", func() string {
		return pick(catchAdjectives) + " " + pick(catchDescriptors) + " " + pick(catchNouns)
	})
	reg("$randomCatchPhraseAdjective", "A random catchphrase adjective", pickF(catchAdjectives))
	reg("$randomCatchPhraseDescriptor", "A random catchphrase descriptor", pickF(catchDescriptors))
	reg("$randomCatchPhraseNoun", "Randomly generates a catchphrase noun", pickF(catchNouns))

	reg("$randomDatabaseColumn", "A random database column name", pickF([]string{"id", "title", "name", "email", "phone", "token", "group", "category", "password", "comment", "avatar", "status", "createdAt", "updatedAt"}))
	reg("$randomDatabaseType", "A random database type", pickF([]string{"int", "varchar", "text", "date", "datetime", "tinyint", "smallint", "mediumint", "bigint", "float", "double", "decimal", "boolean", "timestamp", "time", "enum", "blob", "binary", "point", "geometry"}))
	reg("$randomDatabaseCollation", "A random database collation", pickF([]string{"utf8_unicode_ci", "utf8_general_ci", "utf8_bin", "ascii_bin", "ascii_general_ci", "cp1250_bin", "cp1250_general_ci"}))
	reg("$randomDatabaseEngine", "A random database engine", pickF([]string{"InnoDB", "MyISAM", "MEMORY", "CSV", "BLACKHOLE", "ARCHIVE"}))

	reg("$randomDateFuture", "A random future datetime", func() string {
		return jsDate(time.Now().Add(time.Duration(1+randInt(365*24*3600)) * time.Second))
	})
	reg("$randomDatePast", "A random past datetime", func() string {
		return jsDate(time.Now().Add(-time.Duration(1+randInt(365*24*3600)) * time.Second))
	})
	reg("$randomDateRecent", "A random recent datetime", func() string {
		return jsDate(time.Now().Add(-time.Duration(1+randInt(24*3600)) * time.Second))
	})
	reg("$randomWeekday", "A random weekday", pickF([]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}))
	reg("$randomMonth", "A random month", pickF([]string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}))

	reg("$randomDomainName", "A random domain name", domainName)
	reg("$randomDomainSuffix", "A random domain suffix", pickF(domainSuffixes))
	reg("$randomDomainWord", "A random unqualified domain name", domainWord)
	reg("$randomEmail", "A random email address", func() string {
		return userName() + "@" + pick([]string{"gmail.com", "yahoo.com", "hotmail.com"})
	})
	reg("$randomExampleEmail", "A random email address from an \"example\" domain", func() string {
		return userName() + "@" + pick([]string{"example.org", "example.com", "example.net"})
	})
	reg("$randomUserName", "A random username", userName)
	reg("$randomUrl", "A random URL", func() string {
		return pick([]string{"http", "https"}) + "://" + domainName()
	})

	reg("$randomFileName", "A random file name (includes uncommon extensions)", func() string {
		return strings.ReplaceAll(strings.ToLower(loremWords(1+randInt(3))), " ", "_") + "." + fileExtPick(fileExts)
	})
	reg("$randomFileType", "A random file type (includes uncommon file types)", pickF([]string{"video", "audio", "image", "text", "application", "model", "font", "message", "x-shader", "chemical", "multipart"}))
	reg("$randomFileExt", "A random file extension (includes uncommon extensions)", pickF(fileExts))
	reg("$randomCommonFileName", "A random file name", func() string {
		return strings.ReplaceAll(strings.ToLower(loremWords(1+randInt(3))), " ", "_") + "." + fileExtPick(commonFileExts)
	})
	reg("$randomCommonFileType", "A random, common file type", pickF([]string{"video", "audio", "image", "text", "application"}))
	reg("$randomCommonFileExt", "A random, common file extension", pickF(commonFileExts))
	reg("$randomFilePath", "A random file path", func() string {
		return pick(directoryPaths) + "/" + strings.ReplaceAll(strings.ToLower(loremWords(1+randInt(2))), " ", "_") + "." + fileExtPick(fileExts)
	})
	reg("$randomDirectoryPath", "A random directory path", pickF(directoryPaths))
	reg("$randomMimeType", "A random MIME type", pickF(mimeTypes))

	reg("$randomPrice", "A random price between 0.00 and 1000.00", func() string {
		return strconv.FormatFloat(rand.Float64()*1000, 'f', 2, 64)
	})
	reg("$randomProduct", "A random product", pickF(products))
	reg("$randomProductAdjective", "A random product adjective", pickF(productAdjectives))
	reg("$randomProductMaterial", "A random product material", pickF(productMaterials))
	reg("$randomProductName", "A random product name", func() string {
		return pick(productAdjectives) + " " + pick(productMaterials) + " " + pick(products)
	})
	reg("$randomDepartment", "A random commerce category", pickF(departments))

	reg("$randomNoun", "A random noun", pickF(nouns))
	reg("$randomVerb", "A random verb", pickF(verbs))
	reg("$randomIngverb", "A random verb ending in -ing", pickF(ingVerbs))
	reg("$randomAdjective", "A random adjective", pickF(adjectives))
	reg("$randomWord", "A random word", func() string {
		return pick([][]string{nouns, verbs, adjectives, abbreviations, bsNouns}[randInt(5)])
	})
	reg("$randomWords", "Some random words", func() string {
		n := 1 + randInt(3)
		parts := make([]string, n)
		for i := range parts {
			parts[i] = pick([][]string{nouns, verbs, adjectives, abbreviations, bsNouns}[randInt(5)])
		}
		return strings.Join(parts, " ")
	})
	reg("$randomPhrase", "A random phrase", func() string {
		return phraseTemplates[randInt(len(phraseTemplates))]()
	})

	reg("$randomLoremWord", "A random word of lorem ipsum text", pickF(lorem))
	reg("$randomLoremWords", "Some random words of lorem ipsum text", func() string { return loremWords(1 + randInt(3)) })
	reg("$randomLoremSentence", "A random sentence of lorem ipsum text", loremSentence)
	reg("$randomLoremSentences", "A random 2 to 6 sentences of lorem ipsum text", func() string { return loremSentences(2 + randInt(5)) })
	reg("$randomLoremParagraph", "A random paragraph of lorem ipsum text", loremParagraph)
	reg("$randomLoremParagraphs", "3 random paragraphs of lorem ipsum text", func() string {
		return loremParagraph() + "\n \r" + loremParagraph() + "\n \r" + loremParagraph()
	})
	reg("$randomLoremText", "A random amount of lorem ipsum text", func() string { return loremSentences(1 + randInt(6)) })
	reg("$randomLoremSlug", "A random lorem ipsum URL slug", func() string { return strings.ReplaceAll(loremWords(3), " ", "-") })
	reg("$randomLoremLines", "1 to 5 random lines of lorem ipsum", func() string {
		n := 1 + randInt(5)
		parts := make([]string, n)
		for i := range parts {
			parts[i] = loremSentence()
		}
		return strings.Join(parts, "\n")
	})
}

var phraseTemplates = []func() string{
	func() string {
		return "If we " + pick(verbs) + " the " + pick(nouns) + ", we can get to the " + pick(abbreviations) + " " + pick(nouns) + " through the " + pick(adjectives) + " " + pick(abbreviations) + " " + pick(nouns) + "!"
	},
	func() string {
		return "We need to " + pick(verbs) + " the " + pick(adjectives) + " " + pick(abbreviations) + " " + pick(nouns) + "!"
	},
	func() string {
		return "Try to " + pick(verbs) + " the " + pick(abbreviations) + " " + pick(nouns) + ", maybe it will " + pick(verbs) + " the " + pick(adjectives) + " " + pick(nouns) + "!"
	},
	func() string {
		return "You can't " + pick(verbs) + " the " + pick(nouns) + " without " + pick(ingVerbs) + " the " + pick(adjectives) + " " + pick(abbreviations) + " " + pick(nouns) + "!"
	},
	func() string {
		return "Use the " + pick(adjectives) + " " + pick(abbreviations) + " " + pick(nouns) + ", then you can " + pick(verbs) + " the " + pick(adjectives) + " " + pick(nouns) + "!"
	},
	func() string {
		return "The " + pick(abbreviations) + " " + pick(nouns) + " is down, " + pick(verbs) + " the " + pick(adjectives) + " " + pick(nouns) + " so we can " + pick(verbs) + " the " + pick(abbreviations) + " " + pick(nouns) + "!"
	},
	func() string {
		return pick(ingVerbs) + " the " + pick(nouns) + " won't do anything, we need to " + pick(verbs) + " the " + pick(adjectives) + " " + pick(abbreviations) + " " + pick(nouns) + "!"
	},
	func() string {
		return "I'll " + pick(verbs) + " the " + pick(adjectives) + " " + pick(abbreviations) + " " + pick(nouns) + ", that should " + pick(nouns) + " the " + pick(abbreviations) + " " + pick(nouns) + "!"
	},
}

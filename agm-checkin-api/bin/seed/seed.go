package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	"johndpete316/agm-checkin-api/internal/db"
)

var firstNames = []string{
	"James", "Mary", "John", "Patricia", "Robert", "Jennifer", "Michael", "Linda",
	"William", "Barbara", "David", "Susan", "Richard", "Jessica", "Joseph", "Sarah",
	"Thomas", "Karen", "Charles", "Lisa", "Christopher", "Nancy", "Daniel", "Betty",
	"Matthew", "Margaret", "Anthony", "Sandra", "Mark", "Ashley", "Emily", "Donna",
	"Andrew", "Carol", "Joshua", "Amanda", "Kevin", "Melissa", "Brian", "Stephanie",
	"Timothy", "Laura", "Jason", "Kathleen", "Ryan", "Angela", "Jacob", "Anna",
	"Nicholas", "Emma", "Eric", "Samantha", "Jonathan", "Christine", "Justin", "Nicole",
	"Brandon", "Helen", "Olivia", "Ethan", "Isabella", "Noah", "Sophia", "Liam",
	"Ava", "Mason", "Mia", "Logan", "Charlotte", "Lucas", "Amelia", "Aiden",
}

var lastNames = []string{
	"Smith", "Johnson", "Williams", "Brown", "Jones", "Garcia", "Miller", "Davis",
	"Rodriguez", "Martinez", "Hernandez", "Lopez", "Gonzalez", "Wilson", "Anderson",
	"Thomas", "Taylor", "Moore", "Jackson", "Martin", "Lee", "Perez", "Thompson",
	"White", "Harris", "Sanchez", "Clark", "Ramirez", "Lewis", "Robinson", "Walker",
	"Young", "Allen", "King", "Wright", "Scott", "Torres", "Nguyen", "Hill", "Flores",
	"Green", "Adams", "Nelson", "Baker", "Hall", "Rivera", "Campbell", "Mitchell",
	"Carter", "Roberts", "Chen", "Kim", "Patel", "Okafor", "Kowalski", "Bergman",
}

var studios = []string{
	"Harmony Music Academy",
	"Crescendo School of Music",
	"Allegro Music Studio",
	"Northside Conservatory",
	"Riverside School of the Arts",
	"Maple Street Music",
	"Belcanto Academy",
	"Summit Music Academy",
	"Vivace Music Studio",
	"Westbrook School of Music",
	"Pacific Arts Conservatory",
	"Meadowlark Music Studio",
}

var teachers = []string{
	"Dr. Patricia Holloway",
	"Mr. James Whitfield",
	"Ms. Karen Osei",
	"Prof. David Nakamura",
	"Mrs. Sandra Reyes",
	"Mr. Christopher Bell",
	"Ms. Angela Thornton",
	"Dr. Michael Chen",
	"Mrs. Laura Fitzgerald",
	"Mr. Steven Park",
	"Ms. Rachel Goldstein",
	"Prof. William Torres",
}

var shirtSizes = []string{"XS", "S", "M", "L", "XL", "XXL"}

// Valid event codes — the current event is glr-2026
var validEvents = []string{"glr-2026", "nat-2025", "glr-2025", "nat-2024"}

// Spread check-ins across 3 event days
var eventDays = []time.Time{
	time.Date(2026, 6, 12, 0, 0, 0, 0, time.Local),
	time.Date(2026, 6, 13, 0, 0, 0, 0, time.Local),
	time.Date(2026, 6, 14, 0, 0, 0, 0, time.Local),
}

func randomCheckinTime(day time.Time, rng *rand.Rand) time.Time {
	minutes := rng.Intn(60 * 10) // within a 10-hour window
	return day.Add(8*time.Hour + time.Duration(minutes)*time.Minute)
}

func randomDOB(rng *rand.Rand) time.Time {
	// Mix of minors (ages 8–17) and adults (18–55), roughly 40/60
	var age int
	if rng.Float32() < 0.40 {
		age = 8 + rng.Intn(10) // 8–17
	} else {
		age = 18 + rng.Intn(38) // 18–55
	}
	baseYear := 2026
	month := time.Month(1 + rng.Intn(12))
	day := 1 + rng.Intn(27)
	return time.Date(baseYear-age, month, day, 0, 0, 0, 0, time.UTC)
}

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}
	database := db.Connect(dsn)
	db.AutoMigrate(database)
	database.Where("1 = 1").Delete(&db.Competitor{})

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	var competitors []db.Competitor
	seen := make(map[string]bool)

	for len(competitors) < 100 {
		first := firstNames[rng.Intn(len(firstNames))]
		last := lastNames[rng.Intn(len(lastNames))]
		key := first + " " + last
		if seen[key] {
			continue
		}
		seen[key] = true

		dob := randomDOB(rng)
		age := 2026 - dob.Year()

		isCheckedIn := rng.Float32() < 0.4
		var checkInDateTime *time.Time
		if isCheckedIn {
			t := randomCheckinTime(eventDays[rng.Intn(len(eventDays))], rng)
			checkInDateTime = &t
		}

		// Competitors under 18 require age/identity validation
		requiresValidation := age < 18

		// If validation isn't required the competitor is considered validated.
		// If it is required, randomly mark some as already validated to simulate
		// staff having processed them ahead of time.
		validated := !requiresValidation
		if requiresValidation {
			validated = rng.Float32() < 0.5
		}

		// Some minors have DOB missing — leave as zero time to represent unknown
		if requiresValidation && rng.Float32() < 0.35 {
			dob = time.Time{}
		}

		email := fmt.Sprintf("%s.%s@example.com",
			strings.ToLower(first),
			strings.ToLower(last),
		)

		competitors = append(competitors, db.Competitor{
			NameFirst:           first,
			NameLast:            last,
			DateOfBirth:         dob,
			RequiresValidation:  requiresValidation,
			Validated:           validated,
			IsCheckedIn:         isCheckedIn,
			CheckInDateTime:     checkInDateTime,
			ShirtSize:           shirtSizes[rng.Intn(len(shirtSizes))],
			Email:               email,
			Teacher:             teachers[rng.Intn(len(teachers))],
			Studio:              studios[rng.Intn(len(studios))],
			LastRegisteredEvent: validEvents[rng.Intn(len(validEvents))],
		})
	}

	result := database.Create(&competitors)
	if result.Error != nil {
		log.Fatal("failed to seed competitors:", result.Error)
	}

	log.Printf("seeded %d competitors", len(competitors))
}

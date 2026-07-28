package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	"gorm.io/gorm/clause"

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

// Seeded events, oldest first. glr-2026 is the current one.
var seedEvents = []db.Event{
	{ID: "nat-2024", Name: "Nationals 2024", StartDate: date(2024, 7, 12), EndDate: date(2024, 7, 14)},
	{ID: "glr-2025", Name: "Great Lakes 2025", StartDate: date(2025, 3, 15), EndDate: date(2025, 3, 17)},
	{ID: "nat-2025", Name: "Nationals 2025", StartDate: date(2025, 7, 11), EndDate: date(2025, 7, 13)},
	{ID: "glr-2026", Name: "Great Lakes 2026", StartDate: date(2026, 3, 14), EndDate: date(2026, 3, 16), IsCurrent: true},
}

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

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
	if err := db.Setup(database); err != nil {
		log.Fatal("database setup failed:", err)
	}
	database.Where("1 = 1").Delete(&db.Competitor{})

	// Roster rows carry a foreign key to events, so the events have to exist first.
	for _, event := range seedEvents {
		if err := database.Clauses(clause.OnConflict{DoNothing: true}).Create(&event).Error; err != nil {
			log.Fatal("failed to seed events:", err)
		}
	}

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

		// Most competitors had their ID checked at a previous event; the rest
		// still need verifying at the desk.
		verified := rng.Float32() < 0.7

		// Some of the unverified have no DOB on file at all.
		if !verified && rng.Float32() < 0.35 {
			dob = time.Time{}
		}

		var verifiedAt *time.Time
		verifiedBy := ""
		if verified {
			when := time.Now().AddDate(0, -(rng.Intn(18) + 1), 0)
			verifiedAt = &when
			verifiedBy = "historical import"
		}

		email := fmt.Sprintf("%s.%s@example.com",
			strings.ToLower(first),
			strings.ToLower(last),
		)

		competitors = append(competitors, db.Competitor{
			NameFirst:     first,
			NameLast:      last,
			DateOfBirth:   dob,
			DobVerifiedAt: verifiedAt,
			DobVerifiedBy: verifiedBy,
			ShirtSize:     shirtSizes[rng.Intn(len(shirtSizes))],
			Email:         email,
			Teacher:       teachers[rng.Intn(len(teachers))],
			Studio:        studios[rng.Intn(len(studios))],
		})
	}

	result := database.Create(&competitors)
	if result.Error != nil {
		log.Fatal("failed to seed competitors:", result.Error)
	}

	// Attendance is what makes a competitor visible to registration staff, so a
	// seeded database is useless without roster rows.
	var roster []db.CompetitorEvent
	for _, c := range competitors {
		roster = append(roster, db.CompetitorEvent{
			CompetitorID: c.ID,
			EventID:      seedEvents[rng.Intn(len(seedEvents))].ID,
		})
	}
	if err := database.Create(&roster).Error; err != nil {
		log.Fatal("failed to seed rosters:", err)
	}

	log.Printf("seeded %d competitors across %d events", len(competitors), len(seedEvents))
}

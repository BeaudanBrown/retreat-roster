# Retreat Roster Management System

## Project Overview

The Retreat Roster Management System is a comprehensive staff scheduling and timesheet application built for a restaurant/hospitality business. It manages weekly work schedules, timesheet tracking, staff administration, and leave requests.

## Core Business Domain

### Primary Functions
- **Roster Management:** Weekly staff scheduling with multiple shift types (Early, Mid, Late, Amelia)
- **Timesheet Tracking:** Staff log actual hours worked with break tracking and approval workflows
- **Staff Administration:** Manage staff profiles, availability preferences, and leave requests
- **Conflict Detection:** Automatic highlighting of scheduling conflicts and availability issues

### Key Business Logic
- **Shift Types:** Early, Mid, Late shifts, plus special "Amelia" shifts (specific service area)
- **Conflict Detection Rules:**
  - Staff availability preferences vs assignments
  - Leave request conflicts
  - Duplicate assignments (same person, multiple shifts same day)
  - Late-to-early conflicts (working late then early next day)
  - Ideal shift quota tracking (over/under assigned shifts)
- **User Roles:** Admin staff vs regular staff with different permissions
- **Trial Staff:** Special designation for new employees with different rules

## Technical Architecture

### Backend (Go 1.22)
- **Clean Architecture:** Separated into models, repositories, and server layers
- **Database:** MongoDB with document-based storage
- **Repository Pattern:** Data access abstraction (recently migrated from direct DB calls)
- **Authentication:** Google OAuth2 integration with session management
- **HTTP Server:** REST-like API endpoints with HTMX integration

### Frontend
- **Server-side Rendered:** HTML templates with Go templating
- **HTMX:** Dynamic updates without full page refreshes
- **Styling:** Tailwind CSS with Flowbite components
- **Responsive:** Mobile-friendly design

### Project Structure
```
cmd/
├── migrate/          # Database migration utilities
├── models/           # Data models and business logic
├── repository/       # Data access layer (recently added)
├── server/          # HTTP handlers and routing
└── utils/           # Shared utilities

www/                 # Frontend HTML templates and assets
patches/             # Database patches and migrations
```

### Key Data Models

#### Staff Member
- Personal information and contact details
- Availability preferences by day/shift type
- Leave requests with approval status
- Admin/trial/kitchen role designations
- Session tokens and user configuration

#### Roster Week
- 7-day schedule with multiple shift slots per day
- Staff assignments to specific time slots
- Live/draft status for publishing schedules
- Conflict highlighting and validation flags

#### Timesheet Entry
- Actual worked hours vs scheduled hours
- Break tracking with start/end times
- Shift categorization and approval workflow
- Weekly reporting and wage calculations

## Current State & Known Issues

### Repository Migration
The project is currently mid-way through a major overhaul moving from direct database calls to a proper repository pattern. Main database models have been migrated, but some functionality (especially roster highlighting/flags) needs restoration.

### Problem Areas
1. **Roster Highlighting System:** Conflict detection and flag highlighting needs restoration after repository migration
2. **HTMX Patterns:** Early implementations use inefficient full-page/large DOM refreshes instead of targeted updates
3. **Main Roster Editing Page:** Contains suboptimal patterns from early development, needs refactoring

### Future Goals
- Multi-venue support with configurable shift types and business rules
- Improved performance and user experience
- Better code maintainability and testing

## Development Guidelines

### CRITICAL RESTRICTIONS
- **NEVER RUN THE SERVER** - User handles all server execution and testing
- **NEVER RUN TESTS** - User manages all test execution
- **NEVER EXECUTE** `go run`, `go test`, or similar commands
- Focus on code analysis, editing, and static improvements only

### Code Patterns
- Follow existing Go conventions and project structure
- Use the repository pattern for all database access
- Maintain HTMX integration for frontend interactivity
- Keep styling consistent with Tailwind CSS classes
- Follow clean architecture principles (models → repositories → server)

### Common Tasks
- Debugging roster conflict detection logic
- Optimizing HTMX target selectors for efficient DOM updates
- Improving database query efficiency through repository layer
- Maintaining feature parity during refactoring

### Key Files to Understand
- `cmd/models/` - Core business logic and data structures
- `cmd/repository/` - Database access layer
- `cmd/server/` - HTTP handlers and routing
- `www/` - Frontend templates and HTMX interactions

## Business Workflow
1. Admin creates weekly roster → assigns staff to shifts based on availability
2. Roster goes live → staff view their assignments  
3. Staff work shifts → log actual hours via timesheet
4. Admin approves timesheets → validates worked hours
5. Reports generated → wage calculations and operational reports

## Multi-venue Considerations
Currently designed for single venue ("Retreat") but planning for multi-venue support:
- Shift types and names need to become configurable
- Business rules and conflict detection may vary by venue
- Staff roles and permissions may differ
- Database schema will need venue separation

This system is purpose-built for restaurant/hospitality operations with specific needs around shift scheduling, time tracking, and staff management.
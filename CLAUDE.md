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

## Current State & Recent Progress

### Repository Migration Status
**✅ COMPLETED:** Repository pattern migration is now complete with roster highlighting system fully restored.

### Recent Accomplishments (Latest Session)
1. **✅ Roster Highlighting System:** Fixed conflict detection and flag highlighting post-repository migration
   - Added missing `HasConflict` method to StaffMember model
   - Removed deprecated `CurrentShifts` field, replaced with dynamic calculation
   - Restored template compatibility for availability filtering

2. **✅ Performance Optimization:** Implemented lightweight shift summary for partial HTMX updates
   - Added `StaffAtLimit` map to RootStruct for efficient shift count tracking
   - Calculate shift limits once per week instead of per day/template
   - Enables partial HTMX updates while maintaining dynamic calculation

3. **✅ Test Coverage:** Added comprehensive test suite
   - Unit tests for all models (RosterWeek, StaffMember, TimesheetEntry)
   - Repository helper tests
   - Server function tests and template struct tests
   - Utility function tests with date/time edge cases

### Remaining Problem Areas
1. **HTMX Patterns:** Some implementations still use inefficient full-page refreshes instead of targeted updates
2. **Main Roster Editing Page:** Contains suboptimal patterns from early development, needs refactoring for more targeted updates

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

#### Core Business Logic
- `cmd/models/staff.go` - StaffMember model, availability, conflicts, leave requests
- `cmd/models/rosterweek.go` - RosterWeek model, scheduling, flags, shift counting
- `cmd/models/timesheet.go` - TimesheetEntry model, shift types, time tracking

#### Data Access Layer  
- `cmd/repository/staff_repository.go` - Staff database operations
- `cmd/repository/rosterweek_repository.go` - Roster database operations and week creation
- `cmd/repository/timesheet_repository.go` - Timesheet database operations

#### HTTP Server & Templates
- `cmd/server/root.go` - Main roster page logic, RootStruct, DayStruct creation
- `cmd/server/server.go` - Template functions, server initialization, session handling
- `www/root.html` - Main roster page template with staff table and week navigation
- `www/rosterDay.html` - Individual day template with shift assignments and conflict highlighting

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

# Technical Patterns & Implementation Notes

## Template Optimization Patterns

### Efficient Data Access in Templates
**StaffAtLimit Pattern:** For data that needs to be calculated once and accessed multiple times in templates:
```go
// In RootStruct - calculate once per week
StaffAtLimit map[uuid.UUID]bool

// In templates - use Go's built-in index function
{{ index $.StaffAtLimit .ID }}
```

**Root Context Access:** Use `$` to access root template context from nested scopes:
```html
{{ range .Staff }}
  <!-- Access root context StaffAtLimit map -->
  {{ if index $.StaffAtLimit .ID }}...{{ end }}
{{ end }}
```

### HTMX Update Strategies
**Current Pattern:** Most updates target `#roster` with `outerHTML` (full page refresh)
**Optimization Goal:** Target specific components for partial updates

## Go Template Functions
Template functions registered in `cmd/server/server.go`:
- `CountShiftsForStaff` - Dynamic shift counting for individual staff
- `index` - Built-in function for map/slice access
- `MakeDayStruct` - Creates day context with staff list and metadata
- Root context (`$`) provides access to week-level data from any scope

## Repository Pattern Implementation
All database operations go through repository interfaces:
- `StaffRepository` - Staff member operations
- `RosterWeekRepository` - Roster and scheduling operations  
- `TimesheetRepository` - Time tracking operations
- `ConfigRepository` - Configuration and versioning

## Testing Architecture
Comprehensive test coverage with patterns:
- **Model tests:** Business logic, validation, BSON marshalling
- **Repository tests:** Helper functions and constructors
- **Server tests:** HTTP handling, session management, struct creation
- **Template tests:** Data transformation and presentation logic

## Git Workflow
- **Pre-commit hooks:** Automatically format Go code with gofmt
- **Commit structure:** Separate commits for different concerns (models, templates, tests, etc.)
- **Message format:** Descriptive titles with bullet-point details

## Performance Considerations
- **Shift Counting:** Calculate once per week at root level, not per day/template
- **Template Data:** Minimize database calls within template rendering
- **HTMX Targets:** Use specific element IDs for partial updates vs full page refreshes

## Code Organization Principles
- **Separation of Concerns:** Models handle business logic, repositories handle data access, server handles HTTP
- **Template Functions:** Keep complex logic in Go functions, templates focus on presentation
- **Dynamic vs Static:** Prefer dynamic calculation over stored fields when data changes frequently

## Development Workflow Notes

### Commit Pattern Examples
Recent successful commit structure:
1. Documentation commits (CLAUDE.md, README updates)
2. Test suite commits (all *_test.go files together)
3. Model/business logic commits (isolated changes to core functionality)
4. Template/server commits (UI and presentation layer changes)
5. Build/infrastructure commits (Makefile, migration scripts)

### Testing Commands
- `make test` - Run full test suite (added in recent session)
- Tests cover: models, repositories, server functions, utilities
- Pre-commit hooks ensure code formatting consistency

### Key Methods Added Recently
- `StaffMember.HasConflict(slot, offset)` - Template-friendly conflict checking
- `RosterWeek.CountShiftsForStaff(staffID)` - Dynamic shift counting
- `StaffAtLimit map[uuid.UUID]bool` - Efficient weekly limit tracking
- Template `index` function usage for map access in templates

### Performance Optimization Patterns Implemented
1. **Week-level calculation** instead of day-level for shift limits
2. **Template context sharing** using `$` root context access  
3. **Dynamic calculation** replacing stored database fields
4. **Efficient map lookup** using Go's built-in `index` template function

This establishes a strong foundation for future HTMX optimization work and multi-venue expansion.
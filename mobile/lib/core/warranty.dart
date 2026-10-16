/// Warranty date rules. Mirrors backend/internal/domain/warranty.go so the add-product form
/// can preview the end date offline; the backend remains the source of truth.
library;

/// Suggested warranty period when the document does not state one (TR statutory minimum).
const int defaultWarrantyMonths = 24;

/// Products whose warranty ends within this many days are "expiring soon".
const int expiringSoonDays = 30;

/// Reminder offsets (days before expiry).
const List<int> reminderOffsets = [90, 30, 7];

/// Warranty status derived from the end date.
enum WarrantyStatus { active, expiringSoon, expired, unknown }

/// Parses the API string form of [WarrantyStatus].
WarrantyStatus warrantyStatusFromApi(String? value) {
  switch (value) {
    case 'active':
      return WarrantyStatus.active;
    case 'expiring_soon':
      return WarrantyStatus.expiringSoon;
    case 'expired':
      return WarrantyStatus.expired;
    default:
      return WarrantyStatus.unknown;
  }
}

/// Strips the time-of-day.
DateTime dateOnly(DateTime d) => DateTime.utc(d.year, d.month, d.day);

/// Adds calendar months, clamping to the last day of the target month
/// (31 Jan + 1 month = 28/29 Feb).
DateTime addMonths(DateTime date, int months) {
  final total = date.month - 1 + months;
  final year =
      date.year + (total ~/ 12) - (total < 0 && total % 12 != 0 ? 1 : 0);
  final month = total % 12 + 1;
  final lastDay = DateTime.utc(year, month + 1, 0).day;
  final day = date.day > lastDay ? lastDay : date.day;
  return DateTime.utc(year, month, day);
}

/// Warranty end date, or null when it cannot be computed.
DateTime? warrantyEnd(DateTime? purchase, int months) {
  if (purchase == null || months <= 0) return null;
  return addMonths(dateOnly(purchase), months);
}

/// Whole days from [today] until [end] (negative once expired).
int daysLeft(DateTime end, DateTime today) {
  return dateOnly(end).difference(dateOnly(today)).inDays;
}

/// Derives the status. The end date itself is still covered.
WarrantyStatus statusOf(DateTime? end, DateTime today) {
  if (end == null) return WarrantyStatus.unknown;
  final left = daysLeft(end, today);
  if (left < 0) return WarrantyStatus.expired;
  if (left <= expiringSoonDays) return WarrantyStatus.expiringSoon;
  return WarrantyStatus.active;
}

/// Turkish-style formatting helpers without the intl dependency.
library;

/// Formats a date as dd.MM.yyyy ("-" for null).
String formatDate(DateTime? d) {
  if (d == null) return '-';
  String two(int v) => v.toString().padLeft(2, '0');
  return '${two(d.day)}.${two(d.month)}.${d.year}';
}

/// Formats a timestamp in local time as dd.MM.yyyy HH:mm ("-" for null).
String formatDateTime(DateTime? d) {
  if (d == null) return '-';
  final l = d.toLocal();
  String two(int v) => v.toString().padLeft(2, '0');
  return '${formatDate(l)} ${two(l.hour)}:${two(l.minute)}';
}

/// Formats an ISO date for the API (yyyy-MM-dd).
String isoDate(DateTime d) {
  String two(int v) => v.toString().padLeft(2, '0');
  return '${d.year}-${two(d.month)}-${two(d.day)}';
}

/// Parses yyyy-MM-dd into a UTC date (null on failure).
DateTime? parseIsoDate(String? s) {
  if (s == null || s.length < 10) return null;
  final parts = s.substring(0, 10).split('-');
  if (parts.length != 3) return null;
  final y = int.tryParse(parts[0]);
  final m = int.tryParse(parts[1]);
  final d = int.tryParse(parts[2]);
  if (y == null || m == null || d == null) return null;
  final date = DateTime.utc(y, m, d);
  if (date.month != m || date.day != d) return null;
  return date;
}

/// Parses dd.MM.yyyy typed by the user.
DateTime? parseTrDate(String s) {
  final parts = s.trim().split(RegExp(r'[./-]'));
  if (parts.length != 3) return null;
  final d = int.tryParse(parts[0]);
  final m = int.tryParse(parts[1]);
  final y = int.tryParse(parts[2]);
  if (d == null || m == null || y == null || y < 1990) return null;
  final date = DateTime.utc(y, m, d);
  if (date.month != m || date.day != d) return null;
  return date;
}

const _currencySymbols = {'TRY': 'TL', 'USD': r'$', 'EUR': '€', 'GBP': '£'};

/// Formats money as "12.499,00 TL".
String formatMoney(double value, [String currency = 'TRY']) {
  final negative = value < 0;
  final fixed = value.abs().toStringAsFixed(2);
  final intPart = fixed.substring(0, fixed.length - 3);
  final frac = fixed.substring(fixed.length - 2);
  final buf = StringBuffer();
  for (var i = 0; i < intPart.length; i++) {
    if (i > 0 && (intPart.length - i) % 3 == 0) buf.write('.');
    buf.write(intPart[i]);
  }
  final symbol = _currencySymbols[currency] ?? currency;
  return '${negative ? '-' : ''}$buf,$frac $symbol';
}

/// Parses user-entered amounts: "12.499,00", "12499.5", "12499".
double? parseMoney(String s) {
  var t = s.trim().replaceAll(' ', '').replaceAll('TL', '').replaceAll('₺', '');
  if (t.isEmpty) return null;
  final lastDot = t.lastIndexOf('.');
  final lastComma = t.lastIndexOf(',');
  if (lastDot >= 0 && lastComma >= 0) {
    if (lastComma > lastDot) {
      t = t.replaceAll('.', '').replaceFirst(',', '.');
    } else {
      t = t.replaceAll(',', '');
    }
  } else if (lastComma >= 0) {
    t = t.replaceFirst(',', '.');
  } else if (lastDot >= 0 && t.length - lastDot - 1 == 3) {
    t = t.replaceAll('.', '');
  }
  final v = double.tryParse(t);
  if (v == null || v < 0) return null;
  return v;
}

/// "16 gün kaldı", "Bugün bitiyor", "12 gün önce bitti".
String formatDaysLeft(int days) {
  if (days > 0) return '$days gün kaldı';
  if (days == 0) return 'Bugün bitiyor';
  return '${-days} gün önce bitti';
}

/// Parses a user-typed amount ("1.250,50", "1250.5", "" → 0); null when invalid
/// or outside 0–1 milyar (the backend limit).
double? parseMoneyInput(String text) {
  var t = text.trim().replaceAll(' ', '');
  if (t.isEmpty) return 0;
  if (t.contains(',')) t = t.replaceAll('.', '').replaceAll(',', '.');
  final v = double.tryParse(t);
  return v == null || v.isNaN || v < 0 || v > 1e9 ? null : v;
}

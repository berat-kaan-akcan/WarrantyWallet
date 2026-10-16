/// Turns a raw barcode/QR payload into the product fields it identifies.
///
/// Handles GS1 element strings (AI 01 = GTIN, AI 21 = serial number) in both the
/// human-readable "(01)…(21)…" form and the raw form with GS (0x1D) separators,
/// retail barcodes (EAN-13, EAN-8, UPC-A with check digit), serial labels such as
/// "S/N: ABC123" or "SERİ NO: …", and URLs carrying `sn`/`serial` query parameters.
class ScannedCode {
  const ScannedCode({required this.raw, this.barcode = '', this.serial = ''});

  final String raw;

  /// GTIN/EAN/UPC or other product code.
  final String barcode;

  /// Device serial number.
  final String serial;

  /// Best single value to search with.
  String get lookupCode => serial.isNotEmpty ? serial : barcode;

  bool get isEmpty => barcode.isEmpty && serial.isEmpty;
}

const _gs = '\u001d';

ScannedCode parseScannedCode(String input) {
  final raw = input.trim();
  if (raw.isEmpty) return ScannedCode(raw: raw);

  final gs1 = _parseGs1(raw);
  if (gs1 != null) return gs1;

  final digits = raw.replaceAll(RegExp(r'\s'), '');
  if (RegExp(r'^\d+$').hasMatch(digits) &&
      const {8, 12, 13, 14}.contains(digits.length) &&
      isValidGtin(digits)) {
    return ScannedCode(raw: raw, barcode: digits);
  }

  final uri = Uri.tryParse(raw);
  if (uri != null && uri.hasScheme && uri.host.isNotEmpty) {
    for (final key in const ['sn', 'serial', 'serialNumber', 'serial_number']) {
      final v = uri.queryParameters[key];
      if (v != null && v.trim().isNotEmpty) {
        return ScannedCode(raw: raw, serial: v.trim());
      }
    }
    return ScannedCode(raw: raw, barcode: _cap(raw));
  }

  final label = RegExp(
    r'(?:S/N|SN|SERIAL(?:\s*NO|\s*NUMBER)?|SER[İI]\s*(?:NO|NUMARASI))\s*[:#.]?\s*([A-Z0-9][A-Z0-9\-]{3,39})',
    caseSensitive: false,
  ).firstMatch(raw);
  if (label != null) {
    return ScannedCode(raw: raw, serial: label.group(1)!.toUpperCase());
  }

  // A single token of letters and digits is most likely a serial label (Code 128/39).
  if (RegExp(r'^[A-Za-z0-9\-]{4,40}$').hasMatch(raw) &&
      RegExp(r'[A-Za-z]').hasMatch(raw)) {
    return ScannedCode(raw: raw, serial: raw.toUpperCase());
  }
  return ScannedCode(raw: raw, barcode: _cap(raw));
}

String _cap(String s) => s.length > 128 ? s.substring(0, 128) : s;

/// GS1 mod-10 check digit validation for GTIN-8/12/13/14.
bool isValidGtin(String digits) {
  if (!RegExp(r'^\d{8,14}$').hasMatch(digits)) return false;
  var sum = 0;
  final body = digits.substring(0, digits.length - 1);
  for (var i = 0; i < body.length; i++) {
    final d = int.parse(body[body.length - 1 - i]);
    sum += i.isEven ? d * 3 : d;
  }
  final check = (10 - sum % 10) % 10;
  return check == int.parse(digits[digits.length - 1]);
}

ScannedCode? _parseGs1(String raw) {
  var s = raw;
  if (s.startsWith(']C1') || s.startsWith(']d2') || s.startsWith(']Q3')) {
    s = s.substring(3); // symbology identifier
  }
  String gtin = '', serial = '';
  if (s.startsWith('(')) {
    for (final m in RegExp(r'\((\d{2,4})\)([^(]+)').allMatches(s)) {
      final ai = m.group(1)!, v = m.group(2)!.trim();
      if (ai == '01') gtin = v;
      if (ai == '21') serial = v;
    }
  } else if (s.startsWith('01') &&
      s.length >= 16 &&
      RegExp(r'^01\d{14}').hasMatch(s)) {
    gtin = s.substring(2, 16);
    var rest = s.substring(16);
    while (rest.isNotEmpty) {
      final ai2 = rest.length >= 2 ? rest.substring(0, 2) : '';
      if (ai2 == '21' || ai2 == '10') {
        final end = rest.indexOf(_gs);
        final v = end < 0 ? rest.substring(2) : rest.substring(2, end);
        if (ai2 == '21') serial = v;
        rest = end < 0 ? '' : rest.substring(end + 1);
      } else if (ai2 == '17' || ai2 == '11' || ai2 == '15') {
        rest = rest.length >= 8 ? rest.substring(8) : ''; // fixed-length dates
      } else {
        break;
      }
    }
  } else {
    return null;
  }
  if (gtin.isEmpty && serial.isEmpty) return null;
  return ScannedCode(raw: raw, barcode: gtin, serial: serial);
}

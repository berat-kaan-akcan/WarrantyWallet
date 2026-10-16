import 'package:flutter_test/flutter_test.dart';
import 'package:warranty_wallet/core/scan_parser.dart';

void main() {
  test('EAN-13 with a valid check digit is a barcode', () {
    final c = parseScannedCode('4548736132610');
    expect(c.barcode, '4548736132610');
    expect(c.serial, isEmpty);
    expect(isValidGtin('4006381333931'), isTrue);
    expect(isValidGtin('4006381333932'), isFalse);
    expect(isValidGtin('96385074'), isTrue); // EAN-8
  });

  test('GS1 human readable form yields GTIN and serial', () {
    final c = parseScannedCode('(01)04548736132610(21)5023417');
    expect(c.barcode, '04548736132610');
    expect(c.serial, '5023417');
    expect(c.lookupCode, '5023417');
  });

  test('raw GS1 with group separator and expiry date', () {
    final c =
        parseScannedCode(']C1010454873613261017281231215023417\u001d10LOT9');
    expect(c.barcode, '04548736132610');
    expect(c.serial, '5023417');
  });

  test('serial labels in English and Turkish', () {
    expect(parseScannedCode('S/N: r9lmqs123456').serial, 'R9LMQS123456');
    expect(parseScannedCode('SERİ NO: 5023417').serial, '5023417');
    expect(
        parseScannedCode('Serial Number #C02XK1JHJG5H').serial, 'C02XK1JHJG5H');
  });

  test('plain alphanumeric token is treated as serial', () {
    expect(parseScannedCode('M1LMQS123').serial, 'M1LMQS123');
  });

  test('URL with serial query parameter', () {
    final c =
        parseScannedCode('https://support.example.com/check?sn=ABC12345&x=1');
    expect(c.serial, 'ABC12345');
    final u = parseScannedCode('https://example.com/p/42');
    expect(u.barcode, 'https://example.com/p/42');
  });

  test('empty and free text', () {
    expect(parseScannedCode('   ').isEmpty, isTrue);
    expect(parseScannedCode('Merhaba dünya').barcode, 'Merhaba dünya');
  });
}

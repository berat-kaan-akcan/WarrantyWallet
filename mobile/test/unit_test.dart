import 'package:flutter_test/flutter_test.dart';
import 'package:warranty_wallet/core/formatters.dart';
import 'package:warranty_wallet/core/warranty.dart';
import 'package:warranty_wallet/models/models.dart';
import 'package:warranty_wallet/services/services.dart';

void main() {
  group('warranty dates', () {
    test('adds months like the backend (end-of-month clamp, leap years)', () {
      expect(warrantyEnd(DateTime.utc(2026, 9, 12), 24), DateTime.utc(2028, 9, 12));
      expect(warrantyEnd(DateTime.utc(2026, 1, 31), 1), DateTime.utc(2026, 2, 28));
      expect(warrantyEnd(DateTime.utc(2028, 1, 31), 1), DateTime.utc(2028, 2, 29));
      expect(warrantyEnd(DateTime.utc(2024, 2, 29), 12), DateTime.utc(2025, 2, 28));
      expect(warrantyEnd(DateTime.utc(2026, 12, 15), 1), DateTime.utc(2027, 1, 15));
      expect(addMonths(DateTime.utc(2026, 3, 31), -1), DateTime.utc(2026, 2, 28));
      expect(addMonths(DateTime.utc(2026, 1, 15), -2), DateTime.utc(2025, 11, 15));
      expect(warrantyEnd(null, 24), isNull);
      expect(warrantyEnd(DateTime.utc(2026, 1, 1), 0), isNull);
    });

    test('status and days left', () {
      final end = DateTime.utc(2026, 12, 31);
      expect(statusOf(end, DateTime.utc(2026, 1, 1)), WarrantyStatus.active);
      expect(statusOf(end, DateTime.utc(2026, 12, 1)), WarrantyStatus.expiringSoon);
      expect(statusOf(end, DateTime.utc(2026, 12, 31)), WarrantyStatus.expiringSoon);
      expect(statusOf(end, DateTime.utc(2027, 1, 1)), WarrantyStatus.expired);
      expect(statusOf(null, DateTime.utc(2027, 1, 1)), WarrantyStatus.unknown);
      expect(daysLeft(end, DateTime.utc(2026, 12, 1)), 30);
      expect(warrantyStatusFromApi('expiring_soon'), WarrantyStatus.expiringSoon);
      expect(warrantyStatusFromApi('nonsense'), WarrantyStatus.unknown);
    });
  });

  group('formatters', () {
    test('money', () {
      expect(formatMoney(12499), '12.499,00 TL');
      expect(formatMoney(999.5, 'EUR'), '999,50 €');
      expect(formatMoney(1234567.891, 'USD'), r'1.234.567,89 $');
      expect(parseMoney('12.499,00'), 12499);
      expect(parseMoney('12499.5'), 12499.5);
      expect(parseMoney('12.499'), 12499);
      expect(parseMoney('499,90 TL'), 499.9);
      expect(parseMoney(''), isNull);
      expect(parseMoney('abc'), isNull);
    });

    test('dates', () {
      expect(formatDate(DateTime.utc(2026, 9, 2)), '02.09.2026');
      expect(formatDate(null), '-');
      expect(isoDate(DateTime.utc(2026, 9, 2)), '2026-09-02');
      expect(parseIsoDate('2026-09-12'), DateTime.utc(2026, 9, 12));
      expect(parseIsoDate('2026-02-31'), isNull);
      expect(parseTrDate('12.09.2026'), DateTime.utc(2026, 9, 12));
      expect(parseTrDate('31.02.2026'), isNull);
      expect(formatDaysLeft(16), '16 gün kaldı');
      expect(formatDaysLeft(0), 'Bugün bitiyor');
      expect(formatDaysLeft(-3), '3 gün önce bitti');
    });
  });

  group('models', () {
    test('product from API json', () {
      final p = Product.fromJson(const {
        'id': 'p1',
        'name': 'ASUS TUF VG27AQ',
        'brand': 'ASUS',
        'category': 'monitor',
        'seller': 'MediaMarkt',
        'purchase_date': '2026-09-12',
        'price': 12499,
        'currency': 'TRY',
        'warranty_months': 24,
        'warranty_end': '2028-09-12',
        'serial_number': '',
        'status': 'active',
        'days_left': 697,
      });
      expect(p.price, 12499.0);
      expect(p.warrantyEnd, DateTime.utc(2028, 9, 12));
      expect(p.status, WarrantyStatus.active);
      expect(p.daysLeft, 697);
      expect(p.isCovered, isTrue);
    });

    test('draft json never sends warranty_end', () {
      final d = ProductDraft(name: ' X ', purchaseDate: DateTime.utc(2026, 1, 31), warrantyMonths: 1);
      final j = d.toJson();
      expect(j['name'], 'X');
      expect(j['purchase_date'], '2026-01-31');
      expect(j.containsKey('warranty_end'), isFalse);
      expect(d.warrantyEndPreview, DateTime.utc(2026, 2, 28));
    });

    test('extraction result falls back to suggested warranty', () {
      final r = ExtractionResult.fromJson(const {
        'fields': <String, dynamic>{
          'product_name': 'DeLonghi Dedica',
          'category': 'small_appliance',
          'purchase_date': '2026-03-12',
          'price': 6250,
          'warranty_months': 0,
        },
        'provider': 'rules',
        'missing': ['warranty_months'],
        'suggested_warranty_months': 24,
      });
      expect(r.draft.warrantyMonths, 24);
      expect(r.warrantyStated, isFalse);
      expect(r.missing, ['warranty_months']);
      expect(r.draft.purchaseDate, DateTime.utc(2026, 3, 12));
    });

    test('dashboard json', () {
      final d = Dashboard.fromJson(const {
        'greeting': 'Günaydın, Berat',
        'total_products': 4,
        'active_warranties': 3,
        'expired': 1,
        'total_spent': {'TRY': 26598.95},
        'by_category': [
          {'category': 'headphones', 'name': 'Kulaklık', 'count': 2, 'total': 9099.95},
        ],
        'upcoming': <Map<String, dynamic>>[],
      });
      expect(d.totalSpent['TRY'], 26598.95);
      expect(d.byCategory.single.count, 2);
      expect(categoryName('headphones'), 'Kulaklık');
      expect(ProblemType.fromApi('screen_lines').label, 'Ekranda çizgiler');
      expect(ServiceStatus.fromApi('in_service'), ServiceStatus.inService);
    });
  });

  group('reminders', () {
    final p = Product(
      id: '3f2b8c1e-1a2b-4c3d-8e9f-0123456789ab',
      name: 'ASUS TUF VG27AQ',
      brand: 'ASUS',
      category: 'monitor',
      warrantyEnd: DateTime.utc(2028, 9, 12),
      status: WarrantyStatus.active,
    );

    test('message matches the spec', () {
      expect(reminderMessage(p, 30), 'ASUS monitörünüzün garantisinin bitmesine 30 gün kaldı.');
      final unknown = Product(id: 'x', name: 'Dedica EC685', category: 'other');
      expect(reminderMessage(unknown, 7), 'Dedica EC685 ürününüzün garantisinin bitmesine 7 gün kaldı.');
    });

    test('plans 90/30/7 at 10:00 and skips past ones', () {
      final all = planReminders(p, DateTime(2026, 10, 16));
      expect(all.length, 3);
      expect(all.first.at, DateTime(2028, 6, 14, 10));
      expect(all.last.at, DateTime(2028, 9, 5, 10));
      final remaining = planReminders(p, DateTime(2028, 8, 20));
      expect(remaining.length, 1);
      expect(planReminders(Product(id: 'x', name: 'n'), DateTime(2026)), isEmpty);
    });

    test('reminder ids are stable and distinct', () {
      final ids = reminderOffsets.map((d) => reminderId(p.id, d)).toSet();
      expect(ids.length, 3);
      expect(reminderId(p.id, 30), reminderId(p.id, 30));
      expect(reminderId(p.id, 30), isNot(reminderId('other', 30)));
    });
  });
}

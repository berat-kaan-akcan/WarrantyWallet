import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/scan_parser.dart';
import 'scan_screen.dart';

/// Scans a barcode/QR/serial label. Replaced by a fake in widget tests.
abstract class CodeScanner {
  Future<ScannedCode?> scan(BuildContext context, {String title});
}

/// Camera scanner (mobile_scanner) with manual entry fallback.
class CameraCodeScanner implements CodeScanner {
  @override
  Future<ScannedCode?> scan(BuildContext context,
      {String title = 'Barkod / QR tara'}) async {
    final raw = await Navigator.of(context).push<String>(
      MaterialPageRoute(builder: (_) => ScanScreen(title: title)),
    );
    if (raw == null) return null;
    final code = parseScannedCode(raw);
    return code.isEmpty ? null : code;
  }
}

final codeScannerProvider = Provider<CodeScanner>((ref) => CameraCodeScanner());

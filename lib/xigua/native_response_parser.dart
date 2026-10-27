import 'dart:convert';
import 'dart:isolate';

import 'package:duanju_app/models.dart';

const _backgroundJsonThreshold = 256 * 1024;
const _backgroundCatalogThreshold = 500;

Future<Map<String, dynamic>> decodeNativeResponse(String encoded) async {
  if (encoded.length < _backgroundJsonThreshold) {
    return jsonDecode(encoded) as Map<String, dynamic>;
  }
  return Isolate.run(() => jsonDecode(encoded) as Map<String, dynamic>);
}

Future<CatalogPage> parseNativeCatalog(Map<String, dynamic> data) async {
  final rows = data['items'] as List?;
  if (rows == null || rows.length < _backgroundCatalogThreshold) {
    return CatalogPage.fromJson(data);
  }
  return Isolate.run(() => CatalogPage.fromJson(data));
}

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../dashboard/dashboard_screen.dart';
import '../products/add_product_screen.dart';
import '../products/product_list_screen.dart';
import '../service/service_history_screen.dart';
import 'home_screen.dart';

/// Bottom-navigation shell with the "Ürün Ekle" action.
class MainShell extends ConsumerStatefulWidget {
  const MainShell({super.key});

  @override
  ConsumerState<MainShell> createState() => _MainShellState();
}

class _MainShellState extends ConsumerState<MainShell> {
  int _index = 0;

  void _goTo(int i) => setState(() => _index = i);

  @override
  Widget build(BuildContext context) {
    final pages = <Widget>[
      HomeScreen(onShowAll: () => _goTo(1)),
      const ProductListScreen(),
      const ServiceHistoryScreen(),
      const DashboardScreen(),
    ];
    return Scaffold(
      body: IndexedStack(index: _index, children: pages),
      floatingActionButton: _index <= 1
          ? FloatingActionButton.extended(
              key: const Key('add_product'),
              onPressed: () => Navigator.of(context).push(
                MaterialPageRoute<void>(
                    builder: (_) => const AddProductScreen()),
              ),
              icon: const Icon(Icons.add_a_photo),
              label: const Text('Ürün Ekle'),
            )
          : null,
      bottomNavigationBar: NavigationBar(
        selectedIndex: _index,
        onDestinationSelected: _goTo,
        destinations: const [
          NavigationDestination(
              icon: Icon(Icons.home_outlined),
              selectedIcon: Icon(Icons.home),
              label: 'Ana Sayfa'),
          NavigationDestination(
            icon: Icon(Icons.inventory_2_outlined),
            selectedIcon: Icon(Icons.inventory_2),
            label: 'Ürünler',
          ),
          NavigationDestination(
              icon: Icon(Icons.build_outlined),
              selectedIcon: Icon(Icons.build),
              label: 'Servis'),
          NavigationDestination(
              icon: Icon(Icons.insights_outlined),
              selectedIcon: Icon(Icons.insights),
              label: 'Özet'),
        ],
      ),
    );
  }
}

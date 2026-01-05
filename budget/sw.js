// Service Worker Version - Increment this to trigger update on client devices
const CACHE_NAME = 'budget-pwa-v4';

// Files to cache for offline access
const ASSETS = [
    './budget.html',
    './manifest.json',
    './icon-192.png',
    './icon-512.png'
];

// Install Event: Cache core assets (Shell)
self.addEventListener('install', (e) => {
    e.waitUntil(
        caches.open(CACHE_NAME).then((cache) => cache.addAll(ASSETS))
    );
});

// Activate Event: Cleanup old caches to ensure latest version is used
self.addEventListener('activate', (e) => {
    e.waitUntil(
        caches.keys().then((keys) => {
            return Promise.all(
                keys.map((key) => {
                    if (key !== CACHE_NAME) {
                        return caches.delete(key);
                    }
                })
            );
        })
    );
});

// Fetch Event: Serve from cache first, then network (Stale-While-Revalidate/Cache-First strategy for shell)
self.addEventListener('fetch', (e) => {
    e.respondWith(
        caches.match(e.request).then((response) => {
            return response || fetch(e.request);
        })
    );
});

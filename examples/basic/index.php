<?php
// Basic index.php — serves as the default entry point.
// Returns a JSON response with server info.

header('Content-Type: application/json');
header('X-Powered-By: go-php-gateway-test');

echo json_encode([
    'status'  => 'ok',
    'php'     => PHP_VERSION,
    'server'  => $_SERVER['SERVER_NAME'] ?? 'unknown',
    'method'  => $_SERVER['REQUEST_METHOD'] ?? 'unknown',
    'uri'     => $_SERVER['REQUEST_URI'] ?? '/',
    'time'    => date('c'),
], JSON_PRETTY_PRINT);

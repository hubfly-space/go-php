<?php
// Environment variable test
header('Content-Type: application/json');

$env = [
    'PATH' => getenv('PATH'),
    'HOME' => getenv('HOME'),
    'USER' => getenv('USER'),
    'PWD' => getenv('PWD'),
    'SERVER_NAME' => $_SERVER['SERVER_NAME'] ?? 'unknown',
    'DOCUMENT_ROOT' => $_SERVER['DOCUMENT_ROOT'] ?? 'unknown',
    'SCRIPT_FILENAME' => $_SERVER['SCRIPT_FILENAME'] ?? 'unknown',
];

// Filter out empty values
$env = array_filter($env, function($v) {
    return $v !== '' && $v !== false;
});

echo json_encode($env, JSON_PRETTY_PRINT);

<?php
// User endpoint with path info
header('Content-Type: application/json');

// Extract user ID from PATH_INFO or QUERY_STRING
$pathInfo = $_SERVER['PATH_INFO'] ?? '';
$queryString = $_SERVER['QUERY_STRING'] ?? '';

// Parse user ID from path like /api/users/123
if (preg_match('/\/(\d+)/', $pathInfo, $matches)) {
    $userId = (int)$matches[1];
    
    // Mock user data
    $users = [
        1 => ['id' => 1, 'name' => 'Alice', 'email' => 'alice@example.com'],
        2 => ['id' => 2, 'name' => 'Bob', 'email' => 'bob@example.com'],
        3 => ['id' => 3, 'name' => 'Charlie', 'email' => 'charlie@example.com'],
    ];
    
    if (isset($users[$userId])) {
        $response = $users[$userId];
    } else {
        http_response_code(404);
        $response = ['error' => 'User not found', 'id' => $userId];
    }
} else {
    // List all users
    $response = [
        ['id' => 1, 'name' => 'Alice'],
        ['id' => 2, 'name' => 'Bob'],
        ['id' => 3, 'name' => 'Charlie'],
    ];
}

echo json_encode($response, JSON_PRETTY_PRINT);

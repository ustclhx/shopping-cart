#!/bin/bash 
export APP_HOST="localhost"
export APP_PORT="10000"
export ITEM_CSV="data/items.csv"
export USER_CSV="data/users.csv"
pytest  tests/test_errors.py tests/test_login.py tests/test_items.py tests/test_carts.py tests/test_orders.py tests/test_stock.py tests/test_pay.py

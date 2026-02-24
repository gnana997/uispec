// Sample JavaScript file for parser testing
function calculateTotal(items) {
  return items.reduce((sum, item) => sum + item.price, 0);
}

class ShoppingCart {
  constructor() {
    this.items = [];
  }

  addItem(item) {
    this.items.push(item);
  }

  getTotal() {
    return calculateTotal(this.items);
  }

  clear() {
    this.items = [];
  }
}

module.exports = { calculateTotal, ShoppingCart };

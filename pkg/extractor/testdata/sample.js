// JavaScript sample file for testing extraction
const { EventEmitter } = require('events');
const logger = require('./logger');

class OrderProcessor extends EventEmitter {
  constructor(config) {
    super();
    this.config = config;
  }

  async processOrder(order) {
    logger.info('Processing order', order.id);
    const validated = this.validateOrder(order);
    if (validated) {
      await this.saveOrder(order);
      this.emit('order:processed', order);
    }
    return validated;
  }

  validateOrder(order) {
    return order && order.id && order.items.length > 0;
  }

  async saveOrder(order) {
    return database.orders.insert(order);
  }
}

function createProcessor(config) {
  return new OrderProcessor(config);
}

module.exports = { OrderProcessor, createProcessor };

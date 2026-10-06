# Privacy

Honk if You're Skylar is a private tool for one vehicle owner. It is not a public service and it is not a Tesla product.

Location, speed, gear, battery, charge state, doors, lock state, driver-seat occupancy, guest mode, the allow-list of drivers, and trip history stay on the owner's server. They are stored in the owner's database. They are not sold and they are not sent to an advertising network.

The owner signs in with Tesla. The app never asks for, sees, or stores the Tesla password. The Fleet API client secret and the virtual-key private key stay on the server. They are not in the iOS app.

Granting `vehicle_location` makes the car show its location-sharing icon. That is Tesla's indicator, and it is expected.

The iPhone receives alerts through local notifications and, if the owner configures it, Apple Push Notification service. The phone stores an app session token in the keychain. It does not store Tesla tokens.

The public Fleet API does not reliably return the in-car profile name. Honk shows the driver seat, guest mode, and the allow-list. It does not invent who is driving. Guest mode with an occupied seat is not treated as Skylar.

The owner can delete the server database and remove the virtual key from the car's Locks screen. Removing the key stops live data and commands.

The same text is published at `https://skylar.snapcollectibles.com/privacy`.
